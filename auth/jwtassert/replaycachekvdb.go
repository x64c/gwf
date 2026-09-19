package jwtassert

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/x64c/gwf/gw/kvdbs"
)

// ReplayCacheKVDB is the ReplayCache whose remembered ids live in a KVDB: one
// key per id, inside a region the app's name carves, so every process running
// the app admits an id at most once between them. What is remembered and for
// how long is the same as in memory — only the place differs.
//
// Admitting is ONE conditional write. The id's key is created only if it does
// not exist, and the store settles that atomically, so of any number of
// callers presenting the same id together exactly one creates it and is
// admitted; the rest are refused. A caller pays one round trip per assertion,
// and an unreachable store yields an error rather than a verdict — an
// assertion that could not be checked has not been cleared.
//
// NOTHING SWEEPS. The key's lifetime is the assertion's own remaining
// validity, so the store forgets an id at the moment a replay of it would be
// refused for being expired anyway. A lifetime shorter than the store's clock
// can represent is stored as one tick instead: remembering an id a moment too
// long can only refuse a replay, while forgetting one early would admit it.
//
// The stored value is the moment the id may be forgotten, in unix nanoseconds.
// Nothing reads it — the verdict is the key's existence alone — but it makes a
// key legible to an operator reading the store.
type ReplayCacheKVDB struct {
	kvdb   kvdbs.DB
	prefix string
}

// NewReplayCacheKVDB builds the cache over kvdb, keeping its keys under
// prefix. The prefix is the region: everything sharing it shares the cache, so
// it carries the app's name, and nothing else of the app may write there.
func NewReplayCacheKVDB(kvdb kvdbs.DB, prefix string) (*ReplayCacheKVDB, error) {
	if kvdb == nil {
		return nil, errors.New("jwtassert.NewReplayCacheKVDB: kvdb required")
	}
	if prefix == "" {
		return nil, errors.New("jwtassert.NewReplayCacheKVDB: prefix required")
	}
	return &ReplayCacheKVDB{kvdb: kvdb, prefix: prefix}, nil
}

var _ ReplayCache = (*ReplayCacheKVDB)(nil)

// Admit creates the id's key and reports true; a call finding that key already
// there reports false. until is the assertion's own expiry, and an id already
// past it is admitted without a write — there is nothing left to protect, the
// same answer the in-memory cache gives.
func (c *ReplayCacheKVDB) Admit(ctx context.Context, id string, until time.Time) (bool, error) {
	now := time.Now()
	if !until.After(now) {
		return true, nil
	}
	lifetime := until.Sub(now)
	if c.kvdb.Clock().ReadDurationFrom(now, lifetime) <= 0 {
		lifetime = c.kvdb.Clock().Precision()
	}
	created, err := c.kvdb.SetValueIfNotExists(ctx, c.prefix+id, strconv.FormatInt(until.UnixNano(), 10), lifetime)
	if err != nil {
		return false, fmt.Errorf("jwtassert replay cache: %w", err)
	}
	return created, nil
}
