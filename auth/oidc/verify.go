package oidc

import (
	"context"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"golang.org/x/oauth2"
)

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
