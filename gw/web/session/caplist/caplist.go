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

	"github.com/x64c/gwf/gw/kvdbs"
)

// EvictOverCap enforces capMax on a session-ID list. If the list size exceeds
// the cap, the oldest sessions are evicted — both their umbrella rows in KVDB
// AND their entries in the list — until the list size is back at the cap.
// No-op if the list is already at or below the cap.
//
// rowKeyPrefix maps a session ID from the list to its umbrella row's KVDB key
// as rowKeyPrefix + sessionID. Derive it from the protocol's own row-key
// function with an empty ID (e.g. UserSessionRowKey("")) so the key format
// stays defined in one place. A plain string, not a function, on purpose: the
// concat inlines, so the eviction loop compiles as the pre-hoist copies did.
//
// Caller MUST hold the session lock for listKey before calling. Without the
// lock, a concurrent session-create can push a new sid between the Len read
// and the Trim, evicting the wrong entries.
//
// Access/refresh token rows of evicted sessions are intentionally not deleted
// here — they become harmless pointer rows whose lookup target (the umbrella)
// is now missing, so requests using them fail at the umbrella-fetch step.
// The store's TTL reclaims their memory naturally; doing it explicitly would
// cost extra reads and deletes per evicted session for no functional benefit.
func EvictOverCap(ctx context.Context, kvdb kvdbs.DB, listKey string, capMax int64, rowKeyPrefix string) error {
	sessionCnt, err := kvdb.ListLen(ctx, listKey)
	if err != nil {
		return err
	}
	if sessionCnt <= capMax {
		return nil
	}

	diff := sessionCnt - capMax
	sessionsToDel, err := kvdb.ListRange(ctx, listKey, 0, diff-1)
	if err != nil {
		return err
	}
	keysToDel := make([]string, 0, len(sessionsToDel))
	for _, sid := range sessionsToDel {
		keysToDel = append(keysToDel, rowKeyPrefix+sid)
	}
	_, _ = kvdb.Delete(ctx, keysToDel...)
	if err = kvdb.ListTrim(ctx, listKey, diff, -1); err != nil {
		return err
	}
	return nil
}
