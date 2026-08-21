package fwupstream

import (
	"context"
	"encoding/json/v2"
	"io"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// RequestWithBearer sends a request to the upstream FW app with the
// caller-supplied accessToken formatted into the Authorization Bearer header.
//
// Header layering (in order):
//  1. Framework defaults (Content-Type: application/json) — caller may override
//  2. Caller's payload.Headers — overwrites defaults
//  3. Framework auth (Client-Id, Authorization: Bearer <accessToken>) — always wins
//
// On success (HTTP 200): returns the response with body NOT consumed (caller closes),
// HTTP 200, nil error.
// On failure: returns nil response, the upstream HTTP status (or a framework-mapped
// status for build/transport failures), and a structured *errs.Error parsed from the
// upstream's response body when available.
func (c *Client) RequestWithBearer(
	ctx context.Context,
	accessToken string,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	// Build the request body fresh from the caller's BodyProvider (if any).
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

	// Layer 1: framework defaults (caller may override).
	req.Header.Set("Content-Type", "application/json")
	// Layer 2: caller's headers.
	if payload != nil {
		// inline maps.Copy
		for k, vs := range payload.Headers {
			req.Header[k] = vs
		}
	}
	// Layer 3: framework auth — always wins via last-write.
	req.Header.Set("Client-Id", c.Conf.ClientID)
	req.Header.Set("Authorization", "Bearer "+accessToken)

	// Wire stdlib's GetBody from BodyProvider so stdlib replays (HTTP redirects,
	// HTTP/2 retries) also rebuild the body fresh.
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
