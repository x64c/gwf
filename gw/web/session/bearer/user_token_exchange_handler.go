package bearer

import (
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// UserTokenExchangeHandler is the HTTP handler that mints a user-bound bearer
// session for the user a verified caller names, and answers its tokens. The
// caller is the authn.VerifiedIdentity in the request context — placed there
// by the gate of whatever method authenticated the request (e.g.
// jwtassert.Gate) — and is the session's client: its Subject must be a
// registered bearer client id, and that client's group must bind "client"
// and "user".
//
// The user comes from the identity's claim named UserClaim, through
// ParseUser, which turns the claim value into the uid string the session
// stores. ParseUser MUST reject any value that names no user; it is the one
// judge of what a user id looks like.
//
// App side registers it like:
//
//	router.Handle("POST <path>", &bearer.UserTokenExchangeHandler{
//	    SessionManager: app.SessionService.BearerSessionManager,
//	    UserClaim:      "<claim>",
//	    ParseUser:      func(v any) (string, error) { ... },
//	})
//
// Answers 200 {"access_token", "expires_in", "token_type": "bearer"} plus
// "refresh_token" when WithRefreshToken is set. Refusals: 500
// DataMissingInContext (no identity in context), 400 BearerUserClaimInvalid
// (claim absent or rejected by ParseUser), 403 BearerClientNotFound (Subject
// not a registered bearer client), 403 BearerSessionShapeMismatch (the
// client's group is not user-bound).
type UserTokenExchangeHandler struct {
	SessionManager   *SessionManager
	UserClaim        string
	ParseUser        func(claim any) (string, error)
	WithRefreshToken bool
}

func (h *UserTokenExchangeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, ok := authn.VerifiedIdentityFromContext(ctx)
	if !ok {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.DataMissingInContext.WithDetail("VerifiedIdentity"))
		return
	}

	claim, ok := id.Claims[h.UserClaim]
	if !ok {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.BearerUserClaimInvalid.WithDetail(h.UserClaim+" claim absent"))
		return
	}
	uid, err := h.ParseUser(claim)
	if err != nil || uid == "" {
		responses.WriteErrorJSON(w, http.StatusBadRequest, errs.BearerUserClaimInvalid.WithDetail(h.UserClaim+" claim names no user").WithCause(err))
		return
	}

	clientConf, ok := h.SessionManager.ClientConfs[id.Subject]
	if !ok {
		responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerClientNotFound.WithDetail("not a registered bearer client: "+id.Subject))
		return
	}
	group := clientConf.Group
	if !bindsUser(group) {
		responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerSessionShapeMismatch.WithDetail("group "+group.Name+" is not user-bound"))
		return
	}

	_, accessToken, refreshToken, err := h.SessionManager.CreateSession(ctx, group, map[string]string{
		"client": clientConf.ID,
		"user":   uid,
	})
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to create session").WithCause(err))
		return
	}

	answer := map[string]any{
		"access_token": accessToken,
		"expires_in":   group.AccessTTL,
		"token_type":   "bearer",
	}
	if h.WithRefreshToken {
		answer["refresh_token"] = refreshToken
	}
	responses.EncodeWriteJSON(w, http.StatusOK, answer)
}

// bindsUser reports whether group's sessions bind both a client and a user.
func bindsUser(group *SessionGroupConf) bool {
	var client, user bool
	for _, b := range group.Binds {
		switch b {
		case "client":
			client = true
		case "user":
			user = true
		}
	}
	return client && user
}
