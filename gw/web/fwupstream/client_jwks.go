package fwupstream

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

// RequestJWKS sends a GET to Conf.JwksURL and returns the raw response.
// Caller owns the body and must close it. Use FetchJWKS for parsed JWKS.
func (c *Client) RequestJWKS(ctx context.Context) (*http.Response, *errs.Error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Conf.JwksURL, nil)
	if err != nil {
		return nil, errs.Upstream.Wrap(err)
	}
	req.Header.Set("Client-Id", c.Conf.ClientID)
	req.Header.Set("Accept", "application/jwk-set+json")
	res, err := c.Do(req)
	if err != nil {
		return nil, errs.Upstream.Wrap(err)
	}
	return res, nil
}

// FetchJWKS calls RequestJWKS and parses the response body into *security.JWKS.
func (c *Client) FetchJWKS(ctx context.Context) (*security.JWKS, *errs.Error) {
	res, resErr := c.RequestJWKS(ctx)
	if resErr != nil {
		return nil, resErr
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode == http.StatusNotFound {
		return nil, errs.Upstream.WithDetail("JWKS not found")
	}
	if res.StatusCode != http.StatusOK {
		return nil, errs.Upstream.WithDetail(fmt.Sprintf("status %d", res.StatusCode))
	}

	var jwks security.JWKS
	if err := json.UnmarshalRead(res.Body, &jwks); err != nil {
		return nil, errs.JSONUnmarshalFailed.Wrap(err)
	}
	return &jwks, nil
}
