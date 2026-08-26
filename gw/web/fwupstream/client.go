package fwupstream

import (
	"net/http"
	"sync"
)

// Client is a process-scoped client for talking to an upstream FW app.
//
// Refresh sideloaders (closures that produce extra body fields for the
// token-refresh request) are stored in a typed registry, keyed by per-
// session-data-type keys defined in each session package. Access via the
// generic SetRefreshSideloader / GetRefreshSideloader primitives; session
// packages wrap these with typed setters/getters (e.g.
// bearer.SetUserRefreshSideloader).
type Client struct {
	*http.Client
	ID   string // internal ID
	Conf *ClientConf

	// Signer authenticates the downstream as a machine client to the upstream;
	// set by the downstream at boot. Required by the user token exchange
	// (ExchangeUserToken, RevokeUserToken); nil otherwise.
	Signer MachineSigner

	refreshSideloaders sync.Map // any-key → any-value; typed via per-session-package wrappers
}

// SetRefreshSideloader stores a typed refresh-sideloader closure under key on
// the Client. Intended for use by session packages providing typed setters;
// not called by app code directly. V is preserved at retrieval time via
// GetRefreshSideloader[V].
func (c *Client) SetRefreshSideloader[V any](key any, val V) {
	c.refreshSideloaders.Store(key, val)
}

// GetRefreshSideloader retrieves a typed refresh-sideloader previously stored
// under key. Returns the zero value and false if no sideloader is registered.
// Intended for use by session packages providing typed getters.
func (c *Client) GetRefreshSideloader[V any](key any) (V, bool) {
	var zero V
	v, ok := c.refreshSideloaders.Load(key)
	if !ok {
		return zero, false
	}
	return v.(V), true
}
