package jwtassert

import (
	"sync"
	"time"
)

// replayWindow remembers assertion ids until they expire, so a captured
// assertion cannot be presented twice inside its validity window. It is
// in-process: a verifier running as several processes needs a shared store
// instead (not provided here).
type replayWindow struct {
	mu        sync.Mutex
	ids       map[string]time.Time // id → moment it may be forgotten
	lastSweep time.Time
}

// admit records id and reports true; a second call for the same live id
// reports false. Expired ids are swept lazily.
func (w *replayWindow) admit(id string, until time.Time) bool {
	now := time.Now()
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.ids == nil {
		w.ids = make(map[string]time.Time)
		w.lastSweep = now
	}
	if until.Before(now) {
		return true // already expired; nothing to remember, and the parse rejected it anyway
	}
	if expiry, seen := w.ids[id]; seen && now.Before(expiry) {
		return false
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
	return true
}
