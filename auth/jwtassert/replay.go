package jwtassert

import (
	"context"
	"sync"
	"time"
)

// ReplayStore remembers assertion ids until they expire, so a captured
// assertion cannot be presented twice inside its validity window.
//
// Admit's error is not a verdict: false means "seen before", an error means
// the store could not answer at all — what a store outside this process has
// to report while it is unreachable. An assertion that could not be checked
// has not been cleared.
type ReplayStore interface {
	Admit(ctx context.Context, id string, until time.Time) (bool, error)
}

// ReplayWindow is the in-process ReplayStore: one process's map, guarded by a
// mutex and swept lazily. Several processes verifying the same clients hold
// one window each, so the same assertion is admitted once per process; making
// the window fleet-wide takes a store they share.
//
// [ToDo] Support CrossProc mode.
type ReplayWindow struct {
	mu        sync.Mutex
	ids       map[string]time.Time // id → moment it may be forgotten
	lastSweep time.Time
}

func NewReplayWindow() *ReplayWindow { return &ReplayWindow{} }

var _ ReplayStore = (*ReplayWindow)(nil)

// Admit records id and reports true; a second call for the same live id
// reports false. Expired ids are swept lazily. The error is always nil — the
// map is this process's own, so there is no answer it can fail to compute —
// and ctx goes unread for the same reason. Both are here because ReplayStore
// is what a Verifier holds, and a store it reaches over a network can fail to
// answer at all.
func (w *ReplayWindow) Admit(ctx context.Context, id string, until time.Time) (bool, error) {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ids == nil {
		w.ids = make(map[string]time.Time)
		w.lastSweep = now
	}
	if until.Before(now) {
		return true, nil // already expired; nothing to remember, and the parse rejected it anyway
	}
	if expiry, seen := w.ids[id]; seen && now.Before(expiry) {
		return false, nil
	}
	w.ids[id] = until
	if now.Sub(w.lastSweep) >= time.Minute {
		for k, expiry := range w.ids {
			if !now.Before(expiry) {
				delete(w.ids, k)
			}
		}
		w.lastSweep = now
	}
	return true, nil
}
