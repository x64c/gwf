package oidc

import (
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// DirectExchangeCallbackHandler is the HTTP handler for a Direct Code Exchange
// flow's callback endpoint: it consumes the flow ticket, exchanges the
// returned code with the provider itself, resolves the verified identity to
// the app's user through Resolve, opens a cookie session, and finishes the
// login (cookie set; redirect to the intended URI or SuccessPath).
//
// App side registers it like:
//
//	router.Handle("GET <callback path>", &oidc.DirectExchangeCallbackHandler{
//	    Provider:    &app.LoginConf.Provider,
//	    RedirectURI: app.LoginConf.RedirectURI,
//	    Flow:        app.AuthnFlow,
//	    Sessions:    app.CookieSessionManager,
//	    Resolve:     users.ResolveUIDStr,
//	    SuccessPath: "<path>",
//	})
//
// Refusals: 403 InvalidFlowTicket · 400 AuthCodeNotFound · 503 IDPUnavailable
// · 401 with the verify error (AuthCodeExchangeFailed, IDTokenInvalid) · 401
// with Resolve's error · 500 KVDB / InternalError (session row / cookie).
type DirectExchangeCallbackHandler struct {
	Provider    *Provider
	RedirectURI string
	Flow        *authn.FlowManager
	Sessions    *cookie.SessionManager
	Resolve     authn.UIDStrResolver
	SuccessPath string
}

func (h *DirectExchangeCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	query := r.URL.Query()

	ticket, resErr := h.Flow.ConsumeTicket(w, r, query.Get("state"))
	if resErr != nil {
		responses.WriteErrorJSON(w, http.StatusForbidden, resErr)
		return
	}
	code := query.Get("code")
	if code == "" {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.AuthCodeNotFound)
		return
	}

	verified, err := h.Provider.VerifyAuthCode(ctx, code, h.RedirectURI, ticket.Nonce, ticket.PKCEVerifier)
	if err != nil {
		e := errs.AsStructured(err, errs.IDTokenInvalid)
		status := http.StatusUnauthorized
		if e.IsSameCode(errs.IDPUnavailable) {
			status = http.StatusServiceUnavailable
		}
		responses.WriteErrorJSON(w, status, e)
		return
	}

	uidStr, resErr := h.Resolve(ctx, verified)
	if resErr != nil {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, resErr)
		return
	}

	sessionID, err := h.Sessions.StoreUserSession(ctx, uidStr)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to create session").WithCause(err))
		return
	}
	if err = cookie.FinishLogin(w, r, h.Sessions, sessionID, h.SuccessPath); err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("failed to set session cookie").WithCause(err))
		return
	}
}
