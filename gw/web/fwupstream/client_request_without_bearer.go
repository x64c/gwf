package fwupstream

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// RequestWithoutBearer sends a request to a public (no-bearer) endpoint of the
// upstream FW app. No Authorization header is set. Client-Id is sent only if
// c.Conf.ClientID is non-empty (best-effort identification for analytics or
// rate-limiting; not required for the public endpoint to respond).
//
// Header layering (in order):
//  1. Framework defaults (Content-Type: application/json) — caller may override
//  2. Caller's payload.Headers — overwrites defaults
//  3. Framework Client-Id (only when c.Conf.ClientID != "") — last write
//
// On success (HTTP 200): returns the response with body NOT consumed (caller closes),
// HTTP 200, nil error.
// On failure: returns nil response, the upstream HTTP status (or a framework-mapped
// status for build/transport failures), and a structured *errs.Error parsed from the
// upstream's response body when available.
func (c *Client) RequestWithoutBearer(
	ctx context.Context,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	var reqBodyReader io.Reader
	if payload != nil && payload.BodyProvider != nil {
		var err error
		reqBodyReader, err = payload.BodyProvider()
		if err != nil {
			return nil, http.StatusBadRequest, errs.Upstream.Wrap(err)
		}
	}

	url := c.Conf.Host + endpoint
	req, err := http.NewRequestWithContext(ctx, method, url, reqBodyReader)
	if err != nil {
		return nil, http.StatusBadRequest, errs.Upstream.Wrap(err)
	}

	// Layer 1: framework defaults.
	req.Header.Set("Content-Type", "application/json")
	// Layer 2: caller's headers.
	if payload != nil {
		// inline maps.Copy
		for k, vs := range payload.Headers {
			req.Header[k] = vs
		}
	}
	// Layer 3: framework identification — Client-Id only when configured.
	if c.Conf.ClientID != "" {
		req.Header.Set("Client-Id", c.Conf.ClientID)
	}

	if payload != nil && payload.BodyProvider != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			r, err := payload.BodyProvider()
			if err != nil {
				return nil, err
			}
			return io.NopCloser(r), nil
		}
	}

	res, err := c.Do(req)
	if err != nil {
		return nil, http.StatusBadGateway, errs.Upstream.Wrap(err)
	}

	if res.StatusCode != http.StatusOK {
		defer func() { _ = res.Body.Close() }()
		var apiErr errs.Error
		if err := json.UnmarshalRead(res.Body, &apiErr); err != nil {
			return nil, res.StatusCode, errs.JSONUnmarshalFailed.WithDetail("failed to unmarshal upstream error").WithCause(err)
		}
		return nil, res.StatusCode, &apiErr
	}

	return res, http.StatusOK, nil
}
