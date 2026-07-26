// Package oauth holds primitives for browser-mediated OAuth flows: state token
// generation/verification, and (future) nonce, PKCE, claim-validation helpers.
//
// Server-side OAuth state binding (this file) uses a short-TTL cookie. Native
// mobile / desktop clients handle their own state in-process and don't need
// these helpers.
package oauth

import (
	"crypto/subtle"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

const (
	StateCookieName     = "__Host-oauth-state" // RFC-6265bis `__Host-` prefix
	StateQueryParamName = "state"
	StateMaxAge         = 600 // 10 minutes
)

// IssueState generates a 256-bit URL-safe random state token, sets it in a
// short-TTL HttpOnly Secure SameSite=Lax cookie bound to the user-agent that
// will receive the OAuth provider's callback, and returns the token for
// inclusion in the authorization URL's `state` query parameter.
func IssueState(w http.ResponseWriter) string {
	token := security.GenerateBase64RawURL(32)
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   StateMaxAge,
	})
	return token
}

// VerifyState reads the state cookie and the callback URL's `state` query
// parameter, constant-time compares them, and clears the cookie regardless
// of result.
//
// Returns nil on match. On any failure (cookie absent, query param absent,
// or mismatch), returns errs.InvalidOAuthState — all three collapse to the
// same client decision: reject the callback.
func VerifyState(w http.ResponseWriter, r *http.Request) *errs.Error {
	defer clearStateCookie(w)
	cookie, err := r.Cookie(StateCookieName)
	if err != nil {
		return errs.InvalidOAuthState.WithDetail("state cookie absent")
	}
	queryState := r.URL.Query().Get(StateQueryParamName)
	if queryState == "" {
		return errs.InvalidOAuthState.WithDetail("state query param absent")
	}
	if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(queryState)) != 1 {
		return errs.InvalidOAuthState.WithDetail("state mismatch")
	}
	return nil
}

func clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     StateCookieName,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // delete
	})
}
