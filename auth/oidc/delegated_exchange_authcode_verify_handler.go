package oidc

import (
	"context"
	"encoding/json/v2"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/bearer"
)

// DelegatedExchangeAuthCodeVerifyHandler is the HTTP handler for a Delegated
// Code Exchange flow's verify endpoint on the auth-server side: it verifies
// an authorization code a registered client obtained — exchanging it with
// the IdP and validating the returned ID token — resolves the verified
// identity to the app's user through Resolve, opens a bearer session for
// the client + user, signs the auth server's own ID token, and answers the
// security.AuthResponseBody the client's fwauthserver.Verifier expects.
//
// The caller is identified by its Client-Id header (a registered bearer
// client); Providers maps each client's NAME to the IdP client it
// initiated with. SignIDToken is the auth server's signer (Core.SignIDToken).
//
// App side registers it like:
//
//	router.Handle("POST <verify path>", &oidc.DelegatedExchangeAuthCodeVerifyHandler{
//	    Providers:   app.OIDCProviders["<idp id>"],
//	    Bearer:      app.BearerSessionManager,
//	    Resolve:     users.ResolveUIDStr,
//	    Issuer:      app.Host,
//	    SignIDToken: app.SignIDToken,
//	    IDTokenTTL:  15 * time.Minute,
//	}, <BearerSession gate>, <BodyLimit>)
//
// Refusals: 401 BearerClientNotFound · 500 InternalError (no provider for
// the client) · 400 JSONUnmarshalFailed · 401 AuthClientMismatch · 503
// IDPUnavailable · 401 with the verify error (AuthCodeExchangeFailed,
// IDTokenInvalid) · 401 with Resolve's error · 500 KVDB / InternalError
// (session, signing).
type DelegatedExchangeAuthCodeVerifyHandler struct {
	Providers   map[string]*Provider
	Bearer      *bearer.SessionManager
	Resolve     authn.UIDStrResolver
	Issuer      string
	SignIDToken func(ctx context.Context, iss, sub, email, aud string, ttl time.Duration) (string, error)
	IDTokenTTL  time.Duration
}

func (h *DelegatedExchangeAuthCodeVerifyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	ctx := r.Context()

	clientID := r.Header.Get("Client-Id")
	clientConf, ok := h.Bearer.ClientConfs[clientID]
	if !ok {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.BearerClientNotFound.WithDetail("unknown client_id: "+clientID))
		return
	}
	provider, ok := h.Providers[clientConf.Name]
	if !ok {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("no identity provider configured for client "+clientConf.Name))
		return
	}

	var req security.AuthRequestBody
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.JSONUnmarshalFailed.WithCause(err))
		return
	}
	if req.AuthClientID != provider.ClientID {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.AuthClientMismatch)
		return
	}

	verified, err := provider.VerifyAuthCode(ctx, req.Code, req.RedirectURI, req.Nonce, req.PKCEVerifier)
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

	_, accessToken, refreshToken, err := h.Bearer.CreateSession(ctx, clientConf.Group, map[string]string{
		"client": clientConf.ID,
		"user":   uidStr,
	})
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to create session").WithCause(err))
		return
	}

	email, _ := verified.Claims["email"].(string)
	idToken, err := h.SignIDToken(ctx, h.Issuer, uidStr, email, clientConf.ID, h.IDTokenTTL)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("failed to sign id_token").WithCause(err))
		return
	}

	responses.EncodeWriteJSON(w, http.StatusOK, security.AuthResponseBody{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(clientConf.Group.AccessTTL),
		TokenType:    "bearer",
		IDToken:      idToken,
	})
}
