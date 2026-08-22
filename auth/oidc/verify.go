package oidc

import (
	"context"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/authn"
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
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: code exchange: %w", err)
	}

	signedIDToken, ok := token.Extra("id_token").(string)
	if !ok || signedIDToken == "" {
		return authn.VerifiedIdentity{}, errors.New("oidc: no id_token in token response")
	}

	encodedHeader, _, _, err := security.SplitSignedJwtTokenRawParts(signedIDToken)
	if err != nil {
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: malformed id_token: %w", err)
	}
	headerBytes, err := security.DecodeJwtHeader(encodedHeader)
	if err != nil {
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: malformed id_token header: %w", err)
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(headerBytes, &header); err != nil {
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: malformed id_token header: %w", err)
	}
	if header.Kid == "" {
		return authn.VerifiedIdentity{}, errors.New("oidc: id_token header has no kid")
	}

	pubKey, err := p.keyByKID(ctx, header.Kid)
	if err != nil {
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: signing key: %w", err)
	}

	claims, err := security.VerifyRSASignedIDToken(signedIDToken, pubKey, p.ClientID, p.Issuer)
	if err != nil {
		return authn.VerifiedIdentity{}, fmt.Errorf("oidc: id_token validation: %w", err)
	}

	tokenNonce, ok := claims["nonce"].(string)
	if !ok || subtle.ConstantTimeCompare([]byte(tokenNonce), []byte(nonce)) != 1 {
		return authn.VerifiedIdentity{}, errors.New("oidc: nonce mismatch")
	}

	for claim, want := range p.RequiredClaims {
		got, ok := claims[claim].(string)
		if !ok || got != want {
			return authn.VerifiedIdentity{}, fmt.Errorf("oidc: required claim %q not satisfied", claim)
		}
	}

	if p.RequireEmailVerified {
		verified, ok := claims["email_verified"].(bool)
		if !ok || !verified {
			return authn.VerifiedIdentity{}, errors.New("oidc: email not verified")
		}
	}

	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return authn.VerifiedIdentity{}, errors.New("oidc: id_token has no sub")
	}

	return authn.VerifiedIdentity{
		Method:  Method,
		Subject: sub,
		Claims:  map[string]any(claims),
	}, nil
}
