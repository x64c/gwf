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

var jwksHTTPClient = &http.Client{Timeout: 10 * time.Second}

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
