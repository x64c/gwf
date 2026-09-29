package passwd

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// CookieLoginHandler is the HTTP handler for a password login that opens a cookie session: it
// reads the name and password (a form post or a JSON body, passwd.NameField and
// passwd.PasswordField), verifies them through Verifier, resolves the verified identity to the
// app's user through Resolve, opens a cookie session and finishes the login (cookie set; redirect
// to the intended URI or SuccessPath).
//
// A login form has no session yet to carry a CSRF token, so wire it behind
// handlerwrappers.CheckOriginHeader — a cross-site page cannot post to it. Throttle it per IP and
// per name (passwd.NameThrottleKey) with handlerwrappers.ThrottleFixedGroup, and bound its body.
//
// App side registers it like:
//
//	router.Handle("POST <login path>", &passwd.CookieLoginHandler{
//	    Verifier:    app.PasswordVerifier,
//	    Sessions:    app.CookieSessionManager,
//	    Resolve:     users.ResolveUIDStr,
//	    SuccessPath: "<path>",
//	    FailurePath: "<the form's path>",
//	}, <CheckOriginHeader>, <BodyLimit>, <IP throttle>, <name throttle>)
//
// Refusals — every one either a JSON error or, with FailurePath set, a 303 to it: 400
// InvalidLoginRequest / JSONUnmarshalFailed · 401 InvalidCredentials · 503 PasswordCheckBusy · 401
// with Resolve's error · 500 InternalError / KVDB (lookup, cookie seal, session row). A wrong name and a
// wrong password are one refusal, InvalidCredentials, so none reveals which names exist.
type CookieLoginHandler struct {
	Verifier    *Verifier
	Sessions    *cookie.SessionManager
	Resolve     authn.UIDStrResolver
	SuccessPath string
	FailurePath string // "" → refusals answer a JSON error; else they redirect here (303) and the page says so
}

func (h *CookieLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name, password, e := readLoginRequest(r)
	if e != nil {
		h.refuse(w, r, loginStatus(e), e)
		return
	}
	defer clear(password)

	verified, e := h.Verifier.Verify(ctx, name, password)
	if e != nil {
		h.refuse(w, r, loginStatus(e), e)
		return
	}
	uidStr, e := h.Resolve(ctx, verified)
	if e != nil {
		h.refuse(w, r, http.StatusUnauthorized, e)
		return
	}
	sessionID, sealedCookieValue, err := h.Sessions.NewUserSessionID()
	if err != nil {
		h.refuse(w, r, http.StatusInternalServerError, errs.InternalError.WithDetail("failed to seal session cookie").WithCause(err))
		return
	}
	if err = h.Sessions.StoreUserSession(ctx, sessionID, uidStr, nil); err != nil {
		h.refuse(w, r, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to create session").WithCause(err))
		return
	}
	cookie.FinishLogin(w, r, h.Sessions, sealedCookieValue, h.SuccessPath)
}

// refuse answers a refusal: a 303 to FailurePath when set, else the JSON error. A 5xx is logged
// with its cause either way — the page cannot show it, and the operator must see it.
func (h *CookieLoginHandler) refuse(w http.ResponseWriter, r *http.Request, status int, e *errs.Error) {
	if status >= http.StatusInternalServerError {
		log.Printf("[ERROR][passwd] cookie login: %s: %v", e.Message, e.Cause)
	}
	if h.FailurePath != "" {
		http.Redirect(w, r, h.FailurePath, http.StatusSeeOther)
		return
	}
	responses.WriteErrorJSON(w, status, e)
}
