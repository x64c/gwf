// Package oidc is a relying-party implementation of the OIDC authorization
// code flow with PKCE: building the authorization URL (initiate half) and
// exchanging + validating the returned code into a verified identity
// (verify half). The two halves are independently usable — in a split
// relying party, one app initiates and another verifies.
//
// A Provider value describes one identity provider entirely by
// configuration; this package names none.
package oidc

import (
	"errors"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/security"
)

const Method authn.Method = "oidc"

// Provider describes one OIDC identity provider. Every field is explicit
// configuration — nothing is defaulted. Call Validate at boot.
//
// The zero-value internals carry a JWKS cache; use a Provider as a single
// long-lived *Provider, not by copy.
type Provider struct {
	Issuer   string `json:"issuer"`    // expected `iss` of returned ID tokens
	AuthURL  string `json:"auth_url"`  // authorization endpoint
	TokenURL string `json:"token_url"` // token (code exchange) endpoint
	JWKSURL  string `json:"jwks_url"`  // provider's signing keys

	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"` // "" on the front half of a split relying party

	Scopes          []string          `json:"scopes"`            // explicit; no implied set
	ExtraAuthParams map[string]string `json:"extra_auth_params"` // decorate the authorization URL only; standard parameters win on collision
	RequiredClaims  map[string]string `json:"required_claims"`   // enforced inside the returned ID token

	// RequireEmailVerified rejects ID tokens whose `email_verified` claim is
	// absent or not true. Set when the app resolves users by the email claim.
	RequireEmailVerified bool `json:"require_email_verified"`

	jwksMu        sync.Mutex
	jwks          *security.JWKS
	jwksFetchedAt time.Time
}

// Validate reports the first missing required field. Call at boot; a
// Provider that doesn't validate must not serve.
func (p *Provider) Validate() error {
	if p.Issuer == "" {
		return errors.New("oidc provider: Issuer required")
	}
	if p.AuthURL == "" {
		return errors.New("oidc provider: AuthURL required")
	}
	if p.TokenURL == "" {
		return errors.New("oidc provider: TokenURL required")
	}
	if p.JWKSURL == "" {
		return errors.New("oidc provider: JWKSURL required")
	}
	if p.ClientID == "" {
		return errors.New("oidc provider: ClientID required")
	}
	if len(p.Scopes) == 0 {
		return errors.New("oidc provider: Scopes required")
	}
	return nil
}
