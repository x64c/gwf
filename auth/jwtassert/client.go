package jwtassert

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/security"
)

// Client describes one trusted machine caller, by configuration. NewVerifier
// validates each client and loads its keys at boot.
//
// In configuration, clients are keyed by a human name; ID is the value that
// travels — the assertion's `iss`/`sub`. Name is populated from the
// configuration key for diagnostics.
type Client struct {
	Name string `json:"-"`  // configuration key; diagnostics only
	ID   string `json:"id"` // the assertion's `iss`/`sub`; unique across clients

	Audience     string `json:"audience"`       // the value the client must put in `aud`: the verifying side's identity
	PublicKeyDir string `json:"public_key_dir"` // directory of `<kid>_public.pem` files; the file stem is the kid
	MaxAge       int    `json:"max_age"`        // seconds; `exp` − `iat` may not exceed it
	ClockSkew    int    `json:"clock_skew"`     // seconds of leeway applied to `iat` and `exp`
	MaxBodyBytes int64  `json:"max_body_bytes"` // largest request body the verifier will hash

	jwks *security.JWKS
}

// Validate reports the first missing or non-positive required field.
func (p *Client) Validate() error {
	if p.ID == "" {
		return errors.New("jwtassert client: ID required")
	}
	if p.Audience == "" {
		return errors.New("jwtassert client: Audience required")
	}
	if p.PublicKeyDir == "" {
		return errors.New("jwtassert client: PublicKeyDir required")
	}
	if p.MaxAge <= 0 {
		return errors.New("jwtassert client: MaxAge must be positive")
	}
	if p.ClockSkew < 0 {
		return errors.New("jwtassert client: ClockSkew must not be negative")
	}
	if p.MaxBodyBytes <= 0 {
		return errors.New("jwtassert client: MaxBodyBytes must be positive")
	}
	return nil
}

// LoadKeys reads the client's public keys from PublicKeyDir. A client with no
// loadable key must not serve.
func (p *Client) LoadKeys() error {
	jwks, err := security.LoadPublicPEMKeysAsJWKS(p.PublicKeyDir)
	if err != nil {
		return fmt.Errorf("jwtassert client: %w", err)
	}
	if len(jwks.Keys) == 0 {
		return fmt.Errorf("jwtassert client: no public key in %s", p.PublicKeyDir)
	}
	p.jwks = jwks
	return nil
}
