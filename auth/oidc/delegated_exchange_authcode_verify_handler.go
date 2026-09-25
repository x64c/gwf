package oidc

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
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
// IDTokenInvalid) · 401 with Resolve's error · Admit's status with its error
// (500 InternalError when that status is not 4xx/5xx) · ExtendResponse's
// status with its error (500 InternalError when that status is not 4xx/5xx,
// or when its value is not a JSON object free of the token fields' names) ·
// 500 KVDB / InternalError (session, signing).
type DelegatedExchangeAuthCodeVerifyHandler struct {
	Providers map[string]*Provider
	Bearer    *bearer.SessionManager
	Resolve   authn.UIDStrResolver
	// Admit, when set, decides whether a resolved user may get a session from
	// this endpoint. A nil error admits and the status is ignored; otherwise
	// the error is answered with the status given, which must be 4xx or 5xx —
	// a refusal with its own status, a failure of the check with another.
	// Either way no session opens.
	Admit func(ctx context.Context, uidStr string) (int, *errs.Error)
	// ExtendResponse, when set, returns fields to add to the answer next to
	// the token fields: a value marshaling to a JSON object that names none of
	// them. It runs before the session opens. A nil error uses the value and
	// the status is ignored; otherwise the error is answered with the status
	// given, which must be 4xx or 5xx, and no session opens. Unset, the answer
	// is the token fields alone.
	ExtendResponse func(ctx context.Context, uidStr string) (any, int, *errs.Error)
	Issuer         string
	SignIDToken    func(ctx context.Context, iss, sub, email, aud string, ttl time.Duration) (string, error)
	IDTokenTTL     time.Duration
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

	if h.Admit != nil {
		if status, e := h.Admit(ctx, uidStr); e != nil {
			if status < 400 || status > 599 {
				responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail(fmt.Sprintf("Admit answered status %d with an error", status)).WithCause(e))
				return
			}
			responses.WriteErrorJSON(w, status, e)
			return
		}
	}

	// The extension is fetched and checked before the session opens, so a
	// failed or malformed one leaves no session behind.
	var extension jsontext.Value
	if h.ExtendResponse != nil {
		v, status, e := h.ExtendResponse(ctx, uidStr)
		if e != nil {
			if status < 400 || status > 599 {
				responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail(fmt.Sprintf("ExtendResponse answered status %d with an error", status)).WithCause(e))
				return
			}
			responses.WriteErrorJSON(w, status, e)
			return
		}
		tokenFields, err := json.Marshal(security.AuthResponseBody{})
		if err == nil {
			extension, err = json.Marshal(v)
		}
		if err == nil {
			if _, ok := joinObjects(tokenFields, extension); !ok {
				err = errors.New("not a JSON object free of the token fields' names")
			}
		}
		if err != nil {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("ExtendResponse value").WithCause(err))
			return
		}
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

	body := security.AuthResponseBody{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(clientConf.Group.AccessTTL),
		TokenType:    "bearer",
		IDToken:      idToken,
	}
	if extension == nil {
		responses.EncodeWriteJSON(w, http.StatusOK, body)
		return
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("encoding the answer").WithCause(err))
		return
	}
	joined, _ := joinObjects(bodyJSON, extension) // names checked before the session opened
	responses.EncodeWriteJSON(w, http.StatusOK, joined)
}

// joinObjects returns the members of two JSON objects as one object, and
// false when the result is invalid — a is not an object, b is not an object,
// or they share a member name.
func joinObjects(a, b jsontext.Value) (jsontext.Value, bool) {
	if a.Kind() != '{' || b.Kind() != '{' {
		return nil, false
	}
	if string(b) == "{}" {
		return a, true
	}
	joined := make(jsontext.Value, 0, len(a)+len(b))
	joined = append(joined, a[:len(a)-1]...)
	joined = append(joined, ',')
	joined = append(joined, b[1:]...)
	return joined, joined.IsValid()
}
