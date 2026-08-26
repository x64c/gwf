package fwupstream

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"fmt"
	"io"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// UserTokenExchangeResBody is the upstream's answer to a user token exchange.
type UserTokenExchangeResBody struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// machineRequest sends one request authenticated by c.Signer; body may be
// nil. A transport failure is reported as UpstreamUnavailable.
func (c *Client) machineRequest(ctx context.Context, method, target string, body []byte, extra map[string]any) (*http.Response, *errs.Error) {
	if c.Signer == nil {
		return nil, errs.Upstream.WithDetail("client " + c.ID + " has no machine signer")
	}
	authorization, err := c.Signer.SignRequest(method, target, body, extra)
	if err != nil {
		return nil, errs.Upstream.WithDetail("signing request").WithCause(err)
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.Conf.Host+target, rd)
	if err != nil {
		return nil, errs.Upstream.Wrap(err)
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.Do(req)
	if err != nil {
		return nil, errs.UpstreamUnavailable.Wrap(err)
	}
	return res, nil
}

// ExchangeUserToken asks the upstream's exchange endpoint for a bearer token
// for user, named in the assertion under Conf.UserClaim. Reported failures:
// UpstreamUnavailable (transport, or the upstream's gateway answering 502/504
// for it), Upstream (any other non-200, or a malformed answer).
func (c *Client) ExchangeUserToken(ctx context.Context, user any) (*UserTokenExchangeResBody, *errs.Error) {
	res, resErr := c.machineRequest(ctx, http.MethodPost, c.Conf.TokenExchangeEndpoint, nil, map[string]any{c.Conf.UserClaim: user})
	if resErr != nil {
		return nil, resErr
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusBadGateway || res.StatusCode == http.StatusGatewayTimeout {
		return nil, errs.UpstreamUnavailable.WithDetail("token exchange answered " + res.Status)
	}
	if res.StatusCode != http.StatusOK {
		return nil, errs.Upstream.WithDetail(fmt.Sprintf("token exchange answered %d", res.StatusCode))
	}
	var ans UserTokenExchangeResBody
	if err := json.UnmarshalRead(res.Body, &ans); err != nil || ans.AccessToken == "" {
		return nil, errs.Upstream.WithDetail("malformed token exchange answer")
	}
	return &ans, nil
}

// RevokeUserToken asks the upstream's revocation endpoint to destroy the
// session behind accessToken. Failures as for ExchangeUserToken.
func (c *Client) RevokeUserToken(ctx context.Context, accessToken string) *errs.Error {
	body, err := json.Marshal(map[string]string{"access_token": accessToken})
	if err != nil {
		return errs.JSONMarshalFailed.Wrap(err)
	}
	res, resErr := c.machineRequest(ctx, http.MethodDelete, c.Conf.TokenRevokeEndpoint, body, nil)
	if resErr != nil {
		return resErr
	}
	_ = res.Body.Close()
	if res.StatusCode == http.StatusBadGateway || res.StatusCode == http.StatusGatewayTimeout {
		return errs.UpstreamUnavailable.WithDetail("token revoke answered " + res.Status)
	}
	if res.StatusCode != http.StatusOK {
		return errs.Upstream.WithDetail(fmt.Sprintf("token revoke answered %d", res.StatusCode))
	}
	return nil
}
