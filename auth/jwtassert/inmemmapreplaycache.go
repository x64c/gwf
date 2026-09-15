package jwtassert

import (
	"context"
	"sync"
	"time"
)

// InMemMapReplayCache is the ReplayCache in one process's memory: a map,
// guarded by a mutex and swept lazily. Several processes verifying the same
// clients hold one map each, so the same assertion is admitted once per
// process; admitting it once across them takes a cache they share.
//
// [ToDo] Implement CrossProc version too
type InMemMapReplayCache struct {
	mu        sync.Mutex
	ids       map[string]time.Time // id → moment it may be forgotten
	lastSweep time.Time
}

func NewInMemMapReplayCache() *InMemMapReplayCache { return &InMemMapReplayCache{} }

var _ ReplayCache = (*InMemMapReplayCache)(nil)

// Admit records id and reports true; a second call for the same live id
// reports false. Expired ids are swept lazily. The error is always nil — the
// map is this process's own, so there is no answer it can fail to compute —
// and ctx goes unread for the same reason. Both are here because ReplayCache
// is what a Verifier holds, and a cache it reaches over a network can fail to
// answer at all.
func (c *InMemMapReplayCache) Admit(ctx context.Context, id string, until time.Time) (bool, error) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ids == nil {
		c.ids = make(map[string]time.Time)
		c.lastSweep = now
	}
	if until.Before(now) {
		return true, nil // already expired; nothing to remember, and the parse rejected it anyway
	}
	if expiry, seen := c.ids[id]; seen && now.Before(expiry) {
		return false, nil
	}
	c.ids[id] = until
	if now.Sub(c.lastSweep) >= time.Minute {
		for k, expiry := range c.ids {
			if !now.Before(expiry) {
				delete(c.ids, k)
			}
		}
		c.lastSweep = now
	}
	return true, nil
}
