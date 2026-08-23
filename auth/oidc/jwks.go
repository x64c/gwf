package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

// jwksRefetchMinInterval bounds how often an unknown kid may trigger a
// refetch: within the interval an unknown kid is an error, not a fetch —
// a stream of garbage tokens cannot turn into a request stream at the
// provider.
const jwksRefetchMinInterval = time.Minute

var jwksHTTPClient = &http.Client{Timeout: 10 * time.Second}

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

func publicKeyOf(jwk *security.JWK) (*rsa.PublicKey, *errs.Error) {
	pubKey, err := jwk.ToPublicKey()
	if err != nil {
		return nil, errs.IDPUnavailable.Wrap(fmt.Errorf("jwk: %w", err))
	}
	return pubKey, nil
}

func fetchJWKS(ctx context.Context, jwksURL string) (*security.JWKS, *errs.Error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, errs.IDPUnavailable.Wrap(err)
	}
	res, err := jwksHTTPClient.Do(req)
	if err != nil {
		return nil, errs.IDPUnavailable.Wrap(fmt.Errorf("jwks fetch: %w", err))
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, errs.IDPUnavailable.WithDetail(fmt.Sprintf("jwks fetch: status %d", res.StatusCode))
	}
	var jwks security.JWKS
	if err = json.UnmarshalRead(res.Body, &jwks); err != nil {
		return nil, errs.IDPUnavailable.Wrap(fmt.Errorf("jwks decode: %w", err))
	}
	return &jwks, nil
}
