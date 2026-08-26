package authn

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json/v2"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

const (
	FlowCookieName    = "__Host-authn-flow" // RFC-6265bis `__Host-` prefix
	FlowTicketMaxAge  = 600                 // seconds; a ticket lives one redirect round trip
	FlowCipherPurpose = "authn-flow"        // keyring purpose label the ticket cipher is derived under
)

// FlowManager issues and consumes flow tickets. The cipher seals tickets
// under a context bound to the issuing app's name and the flow cookie: a
// ticket value moved anywhere else stops decrypting.
type FlowManager struct {
	AppName string
	Cipher  security.EncodedCipher
}

// FlowTicket carries the secrets born at a flow's initiate step and consumed
// exactly once at its return step: the state token (callback CSRF binding),
// the nonce (echoed inside the returned identity token), and the PKCE code
// verifier (proven at code exchange). Methods that don't use a field ignore
// it.
type FlowTicket struct {
	State        string `json:"state"`
	Nonce        string `json:"nonce"`
	PKCEVerifier string `json:"pkce_verifier"`
}

// PKCEChallengeS256 returns the S256 code challenge for the ticket's
// verifier: base64url(SHA-256(verifier)), per RFC 7636.
func (t FlowTicket) PKCEChallengeS256() string {
	sum := sha256.Sum256([]byte(t.PKCEVerifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// IssueTicket generates a fresh ticket, sets it as a short-TTL encrypted
// one-shot cookie bound to the user-agent that will carry the flow's return
// request, and returns the ticket for the caller to place its parts into the
// outgoing flow (state and nonce as request parameters, the PKCE verifier as
// its S256 challenge).
func (m *FlowManager) IssueTicket(w http.ResponseWriter) (FlowTicket, error) {
	t := FlowTicket{
		State:        security.GenerateBase64RawURL(32),
		Nonce:        security.GenerateBase64RawURL(32),
		PKCEVerifier: security.GenerateBase64RawURL(32), // 43 chars — RFC 7636 verifier minimum
	}
	plaintext, err := json.Marshal(t)
	if err != nil {
		return FlowTicket{}, err
	}
	sealed, err := m.Cipher.EncryptEncode(plaintext, m.cipherContext())
	if err != nil {
		return FlowTicket{}, err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     FlowCookieName,
		Value:    sealed,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode, // Lax: the cookie must ride the flow's top-level redirect back
		MaxAge:   FlowTicketMaxAge,
	})
	return t, nil
}

// ConsumeTicket reads, decrypts, and deletes the flow ticket cookie, then
// constant-time compares the ticket's state against returnedState (the state
// echoed by the flow's return request). The cookie is cleared regardless of
// result — a ticket is consumable exactly once; a replayed return request
// finds no ticket and fails.
//
// Returns the ticket on match. On any failure (cookie absent, unreadable,
// state absent or mismatched), returns errs.InvalidFlowTicket — all collapse
// to the same caller decision: reject the return request.
func (m *FlowManager) ConsumeTicket(w http.ResponseWriter, r *http.Request, returnedState string) (FlowTicket, *errs.Error) {
	defer clearFlowCookie(w)
	cookie, err := r.Cookie(FlowCookieName)
	if err != nil {
		return FlowTicket{}, errs.InvalidFlowTicket.WithDetail("flow cookie absent")
	}
	if returnedState == "" {
		return FlowTicket{}, errs.InvalidFlowTicket.WithDetail("returned state absent")
	}
	plaintext, err := m.Cipher.DecodeDecrypt(cookie.Value, m.cipherContext())
	if err != nil {
		return FlowTicket{}, errs.InvalidFlowTicket.WithDetail("flow cookie unreadable").WithCause(err)
	}
	var t FlowTicket
	if err = json.Unmarshal(plaintext, &t); err != nil {
		return FlowTicket{}, errs.InvalidFlowTicket.WithDetail("flow cookie malformed").WithCause(err)
	}
	if subtle.ConstantTimeCompare([]byte(t.State), []byte(returnedState)) != 1 {
		return FlowTicket{}, errs.InvalidFlowTicket.WithDetail("state mismatch")
	}
	return t, nil
}

func (m *FlowManager) cipherContext() security.CipherContext {
	return security.CipherContext{App: m.AppName, Location: FlowCookieName}
}

func clearFlowCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     FlowCookieName,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // delete
	})
}
