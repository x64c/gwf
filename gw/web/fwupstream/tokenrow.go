package fwupstream

import (
	"context"
	"net/http"
	"sync"

	"github.com/x64c/gwf/gw/errs"
)

// TokenRow is a row that carries upstream token pairs, seen from this package:
// which Hub owns it, where it lives, where the current request caches its
// token reads, and how to build the refresh request's extra body fields. The
// request ladder below is written once against this constraint.
//
// Implemented by each session-data shape (cookie user/anonymous, bearer
// user/userless) out of what it already holds — its manager and session id.
// The dependency points one way: shapes satisfy this constraint, and this
// package names no session type. A consumer holding rows of its own can
// implement it too.
type TokenRow interface {
	UpstreamHub() *Hub
	UpstreamRowKey() string
	UpstreamAccessSlots() *TknSlots
	UpstreamRefreshSlots() *TknSlots
	// UpstreamRefreshExtras returns the extra body fields for the refresh
	// request, from the sideloader the shape has registered on c, or
	// UpstreamRefreshSideloaderNotSet when it has none.
	UpstreamRefreshExtras(ctx context.Context, c *Client) (map[string]any, *errs.Error)
	// UpstreamRefreshLocker returns the mutual exclusion guarding this row's
	// refresh. Every request touching one row MUST get a locker for the same
	// key from the same store — derive it from the row key, as the address to
	// write is derived, so the two can never disagree. Any sync.Locker will
	// do: a consumer with one row per process can return a plain *sync.Mutex.
	UpstreamRefreshLocker() sync.Locker
}

// TknSlots caches one request's upstream-token reads, keyed by client id.
// The zero value is ready to use.
//
// Per-request state: the only contention is between goroutines one handler
// spawned, and the map is held just long enough to hand out a slot — the KVDB
// read happens under the slot's own Once, outside this lock.
type TknSlots struct {
	mu    sync.Mutex
	slots map[string]*TknSlot
}

// slot returns the slot for clientID, creating it if absent.
func (s *TknSlots) slot(clientID string) *TknSlot {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots == nil {
		s.slots = make(map[string]*TknSlot, 1)
	}
	if slot, ok := s.slots[clientID]; ok {
		return slot
	}
	slot := &TknSlot{}
	s.slots[clientID] = slot
	return slot
}

// setDone replaces clientID's slot with one already holding val, so the rest
// of this request reads the new token rather than the one it replaced.
func (s *TknSlots) setDone(clientID, val string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.slots == nil {
		s.slots = make(map[string]*TknSlot, 1)
	}
	s.slots[clientID] = NewDoneTknSlot(val)
}

// RowAccessToken returns the upstream access token for clientID, reading it
// from the row on first call and caching the result for the rest of the
// request. Concurrent calls for the same clientID share a single read.
func RowAccessToken[R TokenRow](ctx context.Context, row R, clientID string) (string, *errs.Error) {
	slot := row.UpstreamAccessSlots().slot(clientID)
	slot.Once.Do(func() {
		slot.Val, slot.Err = row.UpstreamHub().FetchAccessToken(ctx, row.UpstreamRowKey(), clientID)
	})
	return slot.Val, slot.Err
}

// RowRefreshToken returns the upstream refresh token for clientID, reading it
// from the row on first call and caching the result for the rest of the
// request. Concurrent calls for the same clientID share a single read.
func RowRefreshToken[R TokenRow](ctx context.Context, row R, clientID string) (string, *errs.Error) {
	slot := row.UpstreamRefreshSlots().slot(clientID)
	slot.Once.Do(func() {
		slot.Val, slot.Err = row.UpstreamHub().FetchRefreshToken(ctx, row.UpstreamRowKey(), clientID)
	})
	return slot.Val, slot.Err
}

// RowRequestWithBearer is the foundation call: send a request carrying the
// row's upstream access token. Returns the raw *http.Response (caller closes
// body) on success. res.Body is io.ReadCloser — the byte stream; consume or
// pass through.
func RowRequestWithBearer[R TokenRow](
	ctx context.Context,
	row R,
	c *Client,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	accessTkn, resErr := RowAccessToken(ctx, row, c.ID)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	return c.RequestWithBearer(ctx, accessTkn, method, endpoint, payload)
}

// RowRequestWithBearerRetriable calls RowRequestWithBearer; on a
// refresh-worthy auth failure (token missing locally, or upstream rejected
// it), refreshes the token pair via c.RequestRefreshAccessTknPair, updates the
// row + this request's cached slots, and retries once.
func RowRequestWithBearerRetriable[R TokenRow](
	ctx context.Context,
	row R,
	c *Client,
	method, endpoint string,
	payload *RequestPayload,
) (*http.Response, int, *errs.Error) {
	res, status, resErr := RowRequestWithBearer(ctx, row, c, method, endpoint, payload)
	if resErr == nil {
		return res, status, nil
	}

	shouldRefresh := resErr.IsSameCode(errs.UpstreamAccessTokenNotFound) ||
		resErr.IsSameCode(errs.AccessTokenNotFound) ||
		resErr.IsSameCode(errs.InvalidAccessToken)
	if !shouldRefresh {
		return nil, status, resErr
	}

	// The access token this request just failed with, from its own slot — the
	// value the check below is against.
	staleAccess, _ := RowAccessToken(ctx, row, c.ID)

	// One refresh at a time per row. An access token's expiry is a deadline
	// shared by every request on the session, so they all reach this point at
	// once; unguarded, each spends the same refresh token, and against an
	// upstream that rotates them (gwf's own bearer side does) every spend but
	// one is a replay — leaving the row holding a retired token and the next
	// refresh reading as theft.
	locker := row.UpstreamRefreshLocker()
	locker.Lock()
	defer locker.Unlock()

	// Whoever waited here may find the work already done. Re-read the row —
	// not this request's cached slot, whose whole purpose is to NOT change —
	// and compare against the token that failed: a different one means
	// another request refreshed while this goroutine was blocked. Adopt its
	// pair and retry, rather than spend a refresh token a second time.
	hub, rowKey := row.UpstreamHub(), row.UpstreamRowKey()
	if freshAccess, freshErr := hub.FetchAccessToken(ctx, rowKey, c.ID); freshErr == nil && freshAccess != staleAccess {
		row.UpstreamAccessSlots().setDone(c.ID, freshAccess)
		if storedRefresh, storedErr := hub.FetchRefreshToken(ctx, rowKey, c.ID); storedErr == nil {
			row.UpstreamRefreshSlots().setDone(c.ID, storedRefresh)
		}
		return RowRequestWithBearer(ctx, row, c, method, endpoint, payload)
	}

	refreshTkn, resErr := RowRefreshToken(ctx, row, c.ID)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	refreshExtra, resErr := row.UpstreamRefreshExtras(ctx, c)
	if resErr != nil {
		return nil, http.StatusInternalServerError, resErr
	}
	newPair, resErr := c.RequestRefreshAccessTknPair(ctx, refreshTkn, refreshExtra)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	if resErr := hub.StoreTokenPair(ctx, rowKey, c.ID, newPair.AccessToken, newPair.RefreshToken); resErr != nil {
		return nil, http.StatusInternalServerError, resErr
	}

	// Update cached lazy slots so the retry picks up the new tokens.
	row.UpstreamAccessSlots().setDone(c.ID, newPair.AccessToken)
	row.UpstreamRefreshSlots().setDone(c.ID, newPair.RefreshToken)

	return RowRequestWithBearer(ctx, row, c, method, endpoint, payload)
}
