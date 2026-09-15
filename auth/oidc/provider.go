package oidc

import (
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"golang.org/x/oauth2"
)

const Method authn.Method = "oidc"

// jwksRefetchMinInterval bounds how often an unknown kid may trigger a
// refetch: within the interval an unknown kid is an error, not a fetch —
// a stream of garbage tokens cannot turn into a request stream at the
// provider.
const jwksRefetchMinInterval = time.Minute

// Provider describes one OIDC identity provider, entirely by configuration.
// Call Validate at boot.
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

// AuthCodeURL builds the authorization URL for the flow a ticket opens:
// state and nonce as parameters, the PKCE verifier as its S256 challenge,
// the provider's scopes and extra parameters. redirectURI is where the
// provider sends the code — in a split relying party it belongs to the
// initiating app, and the verify half must receive the same value.
func (p *Provider) AuthCodeURL(t authn.FlowTicket, redirectURI string) string {
	params := url.Values{}
	for key, value := range p.ExtraAuthParams {
		// extras first: the standard parameters set below win on collision
		params.Set(key, value)
	}
	params.Set("response_type", "code")
	params.Set("client_id", p.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", t.State)
	params.Set("nonce", t.Nonce)
	params.Set("scope", strings.Join(p.Scopes, " "))
	params.Set("code_challenge", t.PKCEChallengeS256())
	params.Set("code_challenge_method", "S256")
	return p.AuthURL + "?" + params.Encode()
}

// keyByKID returns the provider signing key for kid, from the cached JWKS
// when it holds the kid, refetching once on a miss (key rotation).
func (p *Provider) keyByKID(ctx context.Context, kid string) (*rsa.PublicKey, *errs.Error) {
	p.jwksMu.Lock()
	defer p.jwksMu.Unlock()
	if p.jwks != nil {
		if jwk, err := p.jwks.GetJWKByKID(kid); err == nil {
			return publicKeyOf(jwk)
		}
		if time.Since(p.jwksFetchedAt) < jwksRefetchMinInterval {
			return nil, errs.IDTokenInvalid.WithDetail("unknown kid " + kid)
		}
	}
	jwks, e := fetchJWKS(ctx, p.JWKSURL)
	if e != nil {
		return nil, e
	}
	p.jwks = jwks
	p.jwksFetchedAt = time.Now()
	jwk, err := jwks.GetJWKByKID(kid)
	if err != nil {
		return nil, errs.IDTokenInvalid.WithDetail("unknown kid " + kid)
	}
	return publicKeyOf(jwk)
}

// VerifyAuthCode exchanges an authorization code and validates the returned
// ID token into a VerifiedIdentity. It takes every flow input as a
// parameter and never assumes it did the initiating: redirectURI, nonce,
// and pkceVerifier are the values the initiating side issued (in a split
// relying party they arrive with the code).
//
// Validation: signature via the provider's JWKS (RSA-only, `exp` required,
// `aud` = ClientID, `iss` = Issuer), nonce echo, RequiredClaims equality,
// RequireEmailVerified when set. Subject is the token's `sub` — the
// provider-stable identifier; the email claim is data, not identity.
//
// Failures are errs.AuthCodeExchangeFailed (the provider refused the code),
// errs.IDTokenInvalid (detail says which check), or errs.IDPUnavailable
// (token endpoint or JWKS unreachable).
func (p *Provider) VerifyAuthCode(ctx context.Context, code, redirectURI, nonce, pkceVerifier string) (authn.VerifiedIdentity, error) {
	exchangeConf := &oauth2.Config{
		ClientID:     p.ClientID,
		ClientSecret: p.ClientSecret,
		RedirectURL:  redirectURI,
		Scopes:       p.Scopes,
		Endpoint:     oauth2.Endpoint{AuthURL: p.AuthURL, TokenURL: p.TokenURL},
	}
	token, err := exchangeConf.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", pkceVerifier))
	if err != nil {
		if _, refused := errors.AsType[*oauth2.RetrieveError](err); refused {
			return authn.VerifiedIdentity{}, errs.AuthCodeExchangeFailed.Wrap(err)
		}
		return authn.VerifiedIdentity{}, errs.IDPUnavailable.Wrap(err)
	}

	signedIDToken, ok := token.Extra("id_token").(string)
	if !ok || signedIDToken == "" {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("missing from token response")
	}

	encodedHeader, _, _, err := security.SplitSignedJwtTokenRawParts(signedIDToken)
	if err != nil {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("malformed")
	}
	headerBytes, err := security.DecodeJwtHeader(encodedHeader)
	if err != nil {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("malformed header")
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(headerBytes, &header); err != nil || header.Kid == "" {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("kid required")
	}

	pubKey, e := p.keyByKID(ctx, header.Kid)
	if e != nil {
		return authn.VerifiedIdentity{}, e
	}

	claims, err := security.VerifyRSASignedIDToken(signedIDToken, pubKey, p.ClientID, p.Issuer)
	if err != nil {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.Wrap(err)
	}

	tokenNonce, ok := claims["nonce"].(string)
	if !ok || subtle.ConstantTimeCompare([]byte(tokenNonce), []byte(nonce)) != 1 {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("nonce mismatch")
	}

	for claim, want := range p.RequiredClaims {
		got, ok := claims[claim].(string)
		if !ok || got != want {
			return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("required claim " + claim + " not satisfied")
		}
	}

	if p.RequireEmailVerified {
		verified, ok := claims["email_verified"].(bool)
		if !ok || !verified {
			return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("email not verified")
		}
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return authn.VerifiedIdentity{}, errs.IDTokenInvalid.WithDetail("sub required")
	}

	return authn.VerifiedIdentity{
		Method:  Method,
		Subject: sub,
		Claims:  map[string]any(claims),
	}, nil
}
