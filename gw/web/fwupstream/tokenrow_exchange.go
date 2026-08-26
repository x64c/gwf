package fwupstream

import (
	"context"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
)

// RowRequestWithExchangeRetriable sends a request as user through the
// row's exchanged upstream bearer: the token cached on the row, or a fresh
// one from c.ExchangeUserToken stored on the row first. On a 401 it
// exchanges once more and retries once. An exchanged token has no refresh
// token; the row carries no refresh-token field for it.
//
// The status is the upstream's, reported as-is with the classifying error
// (UpstreamUnavailable for an unreachable upstream, or its gateway answering
// for it); what to answer its own users is the caller's.
func RowRequestWithExchangeRetriable[R TokenRow](
	ctx context.Context,
	row R,
	c *Client,
	user any,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	tkn, resErr := RowAccessToken(ctx, row, c.ID)
	if resErr != nil || tkn == "" {
		if tkn, resErr = rowExchangeAndStore(ctx, row, c, user); resErr != nil {
			return nil, http.StatusBadGateway, resErr
		}
	}
	res, status, resErr := c.RequestWithBearer(ctx, tkn, method, endpoint, payload)
	if status == http.StatusUnauthorized {
		if tkn, resErr = rowExchangeAndStore(ctx, row, c, user); resErr != nil {
			return nil, http.StatusBadGateway, resErr
		}
		res, status, resErr = c.RequestWithBearer(ctx, tkn, method, endpoint, payload)
	}
	return res, status, resErr
}

// rowExchangeAndStore exchanges for user and caches the token on the row and
// in this request's slot.
func rowExchangeAndStore[R TokenRow](ctx context.Context, row R, c *Client, user any) (string, *errs.Error) {
	ans, resErr := c.ExchangeUserToken(ctx, user)
	if resErr != nil {
		return "", resErr
	}
	if resErr = row.UpstreamHub().StoreAccessToken(ctx, row.UpstreamRowKey(), c.ID, ans.AccessToken); resErr != nil {
		return "", resErr
	}
	row.UpstreamAccessSlots().setDone(c.ID, ans.AccessToken)
	row.UpstreamRefreshSlots().setDone(c.ID, "")
	return ans.AccessToken, nil
}

// RowForgetExchanged removes the row's exchanged upstream bearer (both token
// fields), revoking it at the upstream first when revoke is set (revocation
// failures are ignored: the token dies at the upstream on its own).
func RowForgetExchanged[R TokenRow](ctx context.Context, row R, c *Client, revoke bool) *errs.Error {
	if revoke {
		if tkn, resErr := RowAccessToken(ctx, row, c.ID); resErr == nil && tkn != "" {
			_ = c.RevokeUserToken(ctx, tkn)
		}
	}
	row.UpstreamAccessSlots().setDone(c.ID, "")
	row.UpstreamRefreshSlots().setDone(c.ID, "")
	return row.UpstreamHub().RemoveTokens(ctx, row.UpstreamRowKey(), c.ID)
}
