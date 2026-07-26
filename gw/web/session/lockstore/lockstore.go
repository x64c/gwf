package lockstore

import (
	"sync"
	"sync/atomic"
	"time"
)

// LockEntry pairs a per-key sync.Mutex with a last-touched timestamp.
// The timestamp records the most recent Acquire on this entry; cleanup uses
// it to skip recently-active entries cheaply.
type LockEntry struct {
	mu              sync.Mutex
	lastTouchedNano atomic.Int64
}

func (e *LockEntry) Lock()   { e.mu.Lock() }
func (e *LockEntry) Unlock() { e.mu.Unlock() }

// LastTouchedNano returns the most recent Acquire timestamp in unix-nano.
func (e *LockEntry) LastTouchedNano() int64 {
	return e.lastTouchedNano.Load()
}

// Store is a typed map of key→*LockEntry with last-touched tracking.
// Both session.Service and session managers (cookie, bearer) hold a pointer
// to one shared Store instance.
type Store struct {
	entries sync.Map // map[string]*LockEntry
}

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// Acquire returns the LockEntry for key (creating it if absent) and updates
// its lastTouched timestamp. Caller is expected to Lock / Unlock on the
// returned entry.
func (s *Store) Acquire(key string) *LockEntry {
	v, _ := s.entries.LoadOrStore(key, &LockEntry{})
	entry := v.(*LockEntry)
	entry.lastTouchedNano.Store(time.Now().UnixNano())
	return entry
}

// Range iterates every (key, entry) in the store. Callback returning false
// stops iteration early.
func (s *Store) Range(f func(key string, entry *LockEntry) bool) {
	s.entries.Range(func(k, v any) bool {
		return f(k.(string), v.(*LockEntry))
	})
}

// Delete removes the entry for key.
func (s *Store) Delete(key string) {
	s.entries.Delete(key)
}
