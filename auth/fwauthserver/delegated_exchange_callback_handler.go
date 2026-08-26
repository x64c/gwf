package fwauthserver

import (
	"errors"
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// DelegatedExchangeCallbackHandler is the HTTP handler for a Delegated Code Exchange
// flow's callback endpoint on the browser-facing side: it consumes the flow
// ticket, forwards the returned code (with the ticket's nonce and PKCE
// verifier) to the auth server through Verifier, resolves the verified
// identity to the app's user through Resolve, opens a cookie session, stores
// the auth server's token pair on the session row, and finishes the login
// (cookie set; redirect to the intended URI or SuccessPath).
//
// AuthClientID is the IdP client id the initiate half used; the auth server
// checks it against its own configuration for this client.
//
// App side registers it like:
//
//	router.Handle("GET <callback path>", &fwauthserver.DelegatedExchangeCallbackHandler{
//	    Verifier:     &fwauthserver.Verifier{Upstream: app.MainUpstreamClient, ProviderID: "<idp id>"},
//	    AuthClientID: app.LoginConf.Provider.ClientID,
//	    RedirectURI:  app.LoginConf.RedirectURI,
//	    Flow:         app.AuthnFlow,
//	    Sessions:     app.CookieSessionManager,
//	    Resolve:      users.ResolveUIDStr,
//	    SuccessPath:  "<path>",
//	})
//
// Refusals: 403 InvalidFlowTicket · 400 AuthCodeNotFound · the auth server's
// own answer, forwarded whole, when it refused (*UpstreamError) · 503
// IDPUnavailable · 401 IDTokenInvalid · 401 with Resolve's error · 500 KVDB /
// InternalError (session row, token pair, cookie).
type DelegatedExchangeCallbackHandler struct {
	Verifier     *Verifier
	AuthClientID string
	RedirectURI  string
	Flow         *authn.FlowManager
	Sessions     *cookie.SessionManager
	Resolve      authn.UIDStrResolver
	SuccessPath  string
}

func (h *DelegatedExchangeCallbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
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

	verified, authRes, err := h.Verifier.VerifyAuthCode(ctx, security.AuthRequestBody{
		AuthClientID: h.AuthClientID,
		Code:         code,
		RedirectURI:  h.RedirectURI,
		Nonce:        ticket.Nonce,
		PKCEVerifier: ticket.PKCEVerifier,
	})
	if err != nil {
		if upErr, ok := errors.AsType[*UpstreamError](err); ok {
			w.WriteHeader(upErr.StatusCode)
			_, _ = w.Write(upErr.Body)
			return
		}
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
	if resErr = h.Sessions.UserStoreUpstreamTokenPair(ctx, sessionID, h.Verifier.Upstream.ID, authRes.AccessToken, authRes.RefreshToken); resErr != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, resErr)
		return
	}
	if err = cookie.FinishLogin(w, r, h.Sessions, sessionID, h.SuccessPath); err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("failed to set session cookie").WithCause(err))
		return
	}
}
