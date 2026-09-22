package jwtassert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/kvdbs"
)

// ReplayCache remembers assertion ids until they expire, so a captured
// assertion cannot be presented twice inside its validity window.
//
// Admit's error is not a verdict: false means "seen before", an error means
// the cache could not answer at all — what a cache outside this process has
// to report while it is unreachable. An assertion that could not be checked
// has not been cleared.
type ReplayCache interface {
	Admit(ctx context.Context, id string, until time.Time) (bool, error)
}

// NewReplayCache builds the cache the app's coordination mode calls for, so
// how far the cache reaches is the deployment's own statement:
//
//	InProc    → ReplayCacheInMemMap, this process's memory; kvdb is not used
//	CrossProc → ReplayCacheKVDB over kvdb, keys under "<appName>:ar:"
//
// Pass the app's own values: its AppName(), CoordMode() and MainKVDB. The
// region carries the app's name because the name and the mode together are
// what make two processes one app: instances running under CrossProc with the
// same name against the same store share one cache, and no other app's ids are
// in it. The region is built here, never by the caller.
func NewReplayCache(appName string, coordMode coord.Mode, kvdb kvdbs.DB) (ReplayCache, error) {
	if appName == "" {
		return nil, errors.New("jwtassert.NewReplayCache: appName required")
	}
	switch coordMode {
	case coord.InProc:
		return NewReplayCacheInMemMap(), nil
	case coord.CrossProc:
		if kvdb == nil {
			return nil, errors.New("jwtassert.NewReplayCache: kvdb required — under CrossProc the replay cache lives in it")
		}
		return NewReplayCacheKVDB(kvdb, appName+":ar:")
	default:
		return nil, fmt.Errorf("jwtassert.NewReplayCache: unknown coordination mode %v", coordMode)
	}
}
