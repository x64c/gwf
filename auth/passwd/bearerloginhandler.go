package passwd

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/bearer"
)

// BearerLoginHandler is the HTTP handler for a password login that opens a bearer session: it
// reads the name and password (a JSON body or a form post, passwd.NameField and
// passwd.PasswordField), verifies them through Verifier, resolves the verified identity to the
// app's user through Resolve, opens a bearer session for the client + user, and answers
// security.AccessTokenResponseBody — or, when SignIDToken is set,
// security.AccessTokenAndIDTokenResponseBody with the app's own ID token.
//
// The caller is identified by its Client-Id header — a registered bearer client, whose group sets
// the session's policy; a client registered under the "" id serves callers that send none (a
// curl user, an agent). Bearer tokens are not attached by a browser on its own, so no CSRF defense
// is needed; throttle it per IP and per name (passwd.NameThrottleKey), and bound its body.
//
// App side registers it like:
//
//	router.Handle("POST <login path>", &passwd.BearerLoginHandler{
//	    Verifier: app.PasswordVerifier,
//	    Bearer:   app.BearerSessionManager,
//	    Resolve:  users.ResolveUIDStr,
//	}, <BodyLimit>, <IP throttle>, <name throttle>)
//
// Refusals: 401 BearerClientNotFound · 400 InvalidLoginRequest / JSONUnmarshalFailed · 401
// InvalidCredentials · 503 PasswordCheckBusy · 401 with Resolve's error · 500 InternalError / KVDB
// (lookup, signing, session). A wrong name and a wrong password are one refusal,
// InvalidCredentials, so none reveals which names exist.
type BearerLoginHandler struct {
	Verifier *Verifier
	Bearer   *bearer.SessionManager
	Resolve  authn.UIDStrResolver
	// SignIDToken, when set, signs the app's own ID token into the answer, with Issuer as its `iss`
	// and IDTokenTTL as its lifetime — for clients that did not check the credentials themselves,
	// as when the app signs users in for other apps. Unset, the answer carries no ID token.
	SignIDToken func(ctx context.Context, iss, sub, email, aud string, ttl time.Duration) (string, error)
	Issuer      string
	IDTokenTTL  time.Duration
}

func (h *BearerLoginHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	ctx := r.Context()

	clientID := r.Header.Get("Client-Id")
	clientConf, ok := h.Bearer.ClientConfs[clientID]
	if !ok {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.BearerClientNotFound.WithDetail("unknown client_id: "+clientID))
		return
	}
	name, password, e := readLoginRequest(r)
	if e != nil {
		responses.WriteErrorJSON(w, loginStatus(e), e)
		return
	}
	defer clear(password)

	verified, e := h.Verifier.Verify(ctx, name, password)
	if e != nil {
		status := loginStatus(e)
		if status >= http.StatusInternalServerError {
			log.Printf("[ERROR][passwd] bearer login: %s: %v", e.Message, e.Cause)
		}
		responses.WriteErrorJSON(w, status, e)
		return
	}
	uidStr, e := h.Resolve(ctx, verified)
	if e != nil {
		responses.WriteErrorJSON(w, http.StatusUnauthorized, e)
		return
	}
	// Signed before the session opens, so a failed signing leaves no session behind — nor evicts
	// one of the user's sessions from a capped group.
	var idToken string
	if h.SignIDToken != nil {
		signed, err := h.SignIDToken(ctx, h.Issuer, uidStr, "", clientConf.ID, h.IDTokenTTL)
		if err != nil {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail("failed to sign id_token").WithCause(err))
			return
		}
		idToken = signed
	}
	_, accessToken, refreshToken, err := h.Bearer.CreateSession(ctx, clientConf.Group, map[string]string{
		"client": clientConf.ID,
		"user":   uidStr,
	})
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to create session").WithCause(err))
		return
	}
	body := security.AccessTokenResponseBody{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int64(clientConf.Group.AccessTTL),
		TokenType:    "bearer",
	}
	if h.SignIDToken == nil {
		responses.EncodeWriteJSON(w, http.StatusOK, body)
		return
	}
	responses.EncodeWriteJSON(w, http.StatusOK, security.AccessTokenAndIDTokenResponseBody{
		AccessTokenResponseBody: body,
		IDToken:                 idToken,
	})
}
