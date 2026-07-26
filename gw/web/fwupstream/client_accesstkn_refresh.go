package fwupstream

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// RequestRefreshAccessTknPair POSTs to the upstream's refresh endpoint with
// refreshToken and optional app-specific extras. Returns the parsed response
// on 2xx, or a structured error parsed from the upstream on non-2xx.
func (c *Client) RequestRefreshAccessTknPair(
	ctx context.Context,
	refreshToken string,
	extra map[string]any,
) (*AccessTknRefreshResBody, *errs.Error) {
	reqBody := AccessTknRefreshReqBody{
		RefreshToken: refreshToken,
		Extra:        extra,
	}
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errs.JSONMarshalFailed.Wrap(err)
	}

	url := c.Conf.Host + c.Conf.RefreshAccessTokenEndpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, errs.Upstream.Wrap(err)
	}
	req.Header.Set("Client-Id", c.Conf.ClientID)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	res, err := c.Do(req)
	if err != nil {
		return nil, errs.Upstream.Wrap(err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode != http.StatusOK {
		var apiErr errs.Error
		if err := json.UnmarshalRead(res.Body, &apiErr); err != nil {
			return nil, errs.JSONUnmarshalFailed.WithDetail(fmt.Sprintf("refresh non-OK status %d, body parse failed", res.StatusCode))
		}
		return nil, &apiErr
	}

	var resBody AccessTknRefreshResBody
	if err := json.UnmarshalRead(res.Body, &resBody); err != nil {
		return nil, errs.JSONUnmarshalFailed.Wrap(err)
	}
	return &resBody, nil
}
