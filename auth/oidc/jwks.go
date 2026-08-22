package oidc

import (
	"context"
	"crypto/rsa"
	"encoding/json/v2"
	"fmt"
	"net/http"
	"time"

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
func (p *Provider) keyByKID(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.jwksMu.Lock()
	defer p.jwksMu.Unlock()
	if p.jwks != nil {
		if jwk, err := p.jwks.GetJWKByKID(kid); err == nil {
			return jwk.ToPublicKey()
		}
		if time.Since(p.jwksFetchedAt) < jwksRefetchMinInterval {
			return nil, fmt.Errorf("kid %q not in provider JWKS", kid)
		}
	}
	jwks, err := fetchJWKS(ctx, p.JWKSURL)
	if err != nil {
		return nil, err
	}
	p.jwks = jwks
	p.jwksFetchedAt = time.Now()
	jwk, err := jwks.GetJWKByKID(kid)
	if err != nil {
		return nil, fmt.Errorf("kid %q not in provider JWKS", kid)
	}
	return jwk.ToPublicKey()
}

func fetchJWKS(ctx context.Context, jwksURL string) (*security.JWKS, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := jwksHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jwks fetch: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch: status %d", res.StatusCode)
	}
	var jwks security.JWKS
	if err = json.UnmarshalRead(res.Body, &jwks); err != nil {
		return nil, fmt.Errorf("jwks decode: %w", err)
	}
	return &jwks, nil
}
