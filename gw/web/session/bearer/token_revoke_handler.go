package bearer

import (
	"encoding/json/v2"
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
)

// TokenRevokeHandler is the HTTP handler that destroys the bearer session
// behind a posted access token, on behalf of the client that owns it. The
// caller is the authn.VerifiedIdentity in the request context (placed by the
// gate of whatever method authenticated the request); a session is revocable
// by its own client only — the session row's client id must equal the
// identity's Subject.
//
// Reads a JSON body {"access_token": "..."} — apply BodyLimit as for the
// refresh handler. Idempotent: an unknown or already-dead token answers
// {"revoked": false}; a destroyed one {"revoked": true}. Refusals: 500
// DataMissingInContext (no identity in context), 400 JSONUnmarshalFailed /
// AccessTokenNotFound (body malformed / token empty), 403
// BearerSessionNotOwned (another client's session).
//
// App side registers it like:
//
//	router.Handle("DELETE <path>", &bearer.TokenRevokeHandler{
//	    SessionManager: app.SessionService.BearerSessionManager,
//	})
type TokenRevokeHandler struct {
	SessionManager *SessionManager
}

type tokenRevokeRequestBody struct {
	AccessToken string `json:"access_token"`
}

func (h *TokenRevokeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	ctx := r.Context()
	id, ok := authn.VerifiedIdentityFromContext(ctx)
	if !ok {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.DataMissingInContext.WithDetail("VerifiedIdentity"))
		return
	}

	var reqBody tokenRevokeRequestBody
	if err := json.UnmarshalRead(r.Body, &reqBody); err != nil {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.JSONUnmarshalFailed.WithCause(err))
		return
	}
	if reqBody.AccessToken == "" {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.AccessTokenNotFound.WithDetail("access_token required in body"))
		return
	}

	m := h.SessionManager
	sid, found, err := m.KVDB.GetValue(ctx, m.AccessTokenRowKey(security.HashHexSHA256(reqBody.AccessToken)))
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("session lookup failed").WithCause(err))
		return
	}
	if !found {
		responses.EncodeWriteJSON(w, http.StatusOK, map[string]bool{"revoked": false})
		return
	}
	row, err := m.FetchSession(ctx, sid)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("session lookup failed").WithCause(err))
		return
	}
	if row == nil {
		responses.EncodeWriteJSON(w, http.StatusOK, map[string]bool{"revoked": false})
		return
	}
	if row.ClientID != id.Subject {
		responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerSessionNotOwned)
		return
	}
	if err = m.DestroySession(ctx, sid); err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to destroy session").WithCause(err))
		return
	}
	responses.EncodeWriteJSON(w, http.StatusOK, map[string]bool{"revoked": true})
}
