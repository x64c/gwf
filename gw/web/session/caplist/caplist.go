// Package caplist holds the capped session-ID list shared by the session
// protocols: the per-principal list a protocol pushes each new session ID
// onto, and the eviction that keeps it at its configured cap.
//
// These are helpers, not requirements. They assume one style of KVDB layout —
// a list of session IDs plus one umbrella row per session at a prefixed key —
// which is the style the built-in protocols use. A session implementation
// with a different layout keeps its own eviction; nothing in the framework
// checks how a cap is enforced.
package caplist

import (
	"context"
	"time"

	"github.com/x64c/gwf/gw/kvdbs"
)

// PushEvictOverCap pushes sid onto the session-ID list at listKey, gives the
// list the lifetime keyTTL, and evicts the oldest sessions over capMax — their
// entries in the list AND their umbrella rows in KVDB. The push, the lifetime,
// and the list trim are one atomic act in the store, so concurrent creates
// never evict each other's sessions and the list is never without a lifetime.
// The umbrella rows are deleted after, which needs no atomicity: an evicted ID
// is never pushed again, and a row that outlives a crash here carries its own
// TTL. No lock is needed around this call.
//
// rowKeyPrefix maps a session ID from the list to its umbrella row's KVDB key
// as rowKeyPrefix + sessionID. Derive it from the protocol's own row-key
// function with an empty ID (e.g. UserSessionRowKey("")) so the key format
// stays defined in one place. A plain string, not a function, on purpose: the
// concat inlines, so the eviction loop compiles as the pre-hoist copies did.
//
// Access/refresh token rows of evicted sessions are intentionally not deleted
// here — they become harmless pointer rows whose lookup target (the umbrella)
// is now missing, so requests using them fail at the umbrella-fetch step.
// The store's TTL reclaims their memory naturally; doing it explicitly would
// cost extra reads and deletes per evicted session for no functional benefit.
func PushEvictOverCap(ctx context.Context, kvdb kvdbs.DB, listKey, sid string, capMax int64, keyTTL time.Duration, rowKeyPrefix string) error {
	evicted, err := kvdb.ListPushTrimOverCap(ctx, listKey, sid, capMax, keyTTL)
	if err != nil {
		return err
	}
	if len(evicted) == 0 {
		return nil
	}
	keysToDel := make([]string, 0, len(evicted))
	for _, evictedSID := range evicted {
		keysToDel = append(keysToDel, rowKeyPrefix+evictedSID)
	}
	_, _ = kvdb.Delete(ctx, keysToDel...)
	return nil
}
