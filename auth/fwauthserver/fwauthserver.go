// Package fwauthserver authenticates against a framework auth server: the
// app forwards an authorization code — with the flow secrets the initiating
// side issued — to the auth server over its upstream client, and verifies
// the auth server's signed ID token against the auth server's JWKS. The
// auth server is this method's identity provider.
package fwauthserver

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"

	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

const Method authn.Method = "fwauthserver"

// Verifier forwards codes to one auth server and verifies its ID tokens.
type Verifier struct {
	Upstream   *fwupstream.Client // the auth server
	ProviderID string             // key into Upstream.Conf.VerifyAuthCodeEndpoints
}

// UpstreamError is a non-200 answer from the auth server, carried whole so
// the caller can forward it.
type UpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *UpstreamError) Error() string { return fmt.Sprintf("auth server answered %d", e.StatusCode) }

// VerifyAuthCode forwards req to the auth server's verify endpoint for
// ProviderID and validates the returned ID token (RSA-only, `exp` required,
// `aud` = this app's client id at the auth server, `iss` = the auth server
// host) against the auth server's JWKS. Subject is the token's `sub`. The
// auth server's token response is returned alongside for the app to keep.
//
// Failures are *UpstreamError (the auth server answered non-200 — forward
// it), errs.IDTokenInvalid (detail says which check), or errs.IDPUnavailable
// (auth server or its JWKS unreachable).
func (v *Verifier) VerifyAuthCode(ctx context.Context, req security.AuthRequestBody) (authn.VerifiedIdentity, *security.AuthResponseBody, error) {
	endpoint, ok := v.Upstream.Conf.VerifyAuthCodeEndpoints[v.ProviderID]
	if !ok {
		return authn.VerifiedIdentity{}, nil, fmt.Errorf("fwauthserver: no verify endpoint for provider %q", v.ProviderID)
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, v.Upstream.Conf.Host+endpoint, bytes.NewReader(reqBytes))
	if err != nil {
		return authn.VerifiedIdentity{}, nil, err
	}
	httpReq.Header.Set("Client-Id", v.Upstream.Conf.ClientID)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	res, err := v.Upstream.Do(httpReq)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDPUnavailable.Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
		return authn.VerifiedIdentity{}, nil, &UpstreamError{StatusCode: res.StatusCode, Body: body}
	}

	var authRes security.AuthResponseBody
	if err = json.UnmarshalRead(res.Body, &authRes); err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDPUnavailable.Wrap(fmt.Errorf("response decode: %w", err))
	}

	encodedHeader, _, _, err := security.SplitSignedJwtTokenRawParts(authRes.IDToken)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.WithDetail("malformed")
	}
	headerBytes, err := security.DecodeJwtHeader(encodedHeader)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.WithDetail("malformed header")
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(headerBytes, &header); err != nil || header.Kid == "" {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.WithDetail("kid required")
	}

	jwks, resErr := v.Upstream.FetchJWKS(ctx)
	if resErr != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDPUnavailable.Wrap(resErr)
	}
	jwk, err := jwks.GetJWKByKID(header.Kid)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.WithDetail("unknown kid " + header.Kid)
	}
	pubKey, err := jwk.ToPublicKey()
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDPUnavailable.Wrap(fmt.Errorf("jwk: %w", err))
	}

	claims, err := security.VerifyRSASignedIDToken(authRes.IDToken, pubKey, v.Upstream.Conf.ClientID, v.Upstream.Conf.Host)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.Wrap(err)
	}
	sub, ok := claims["sub"].(string)
	if !ok || sub == "" {
		return authn.VerifiedIdentity{}, nil, errs.IDTokenInvalid.WithDetail("sub required")
	}

	return authn.VerifiedIdentity{
		Method:  Method,
		Subject: sub,
		Claims:  map[string]any(claims),
	}, &authRes, nil
}
