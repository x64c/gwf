package jwtassert

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/framework"
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
// how far the cache reaches is the deployment's own statement rather than a
// choice made at the call site:
//
//	InProc    → ReplayCacheInMemMap, this process's memory
//	CrossProc → ReplayCacheKVDB over the app's MainKVDB, keys under
//	            "<app name>:ar:"; MainKVDB must be set before this call
//
// The region carries the app's name because the name and the mode together
// are what make two processes one app: instances running under CrossProc with
// the same name against the same store share one cache, and no other app's
// ids are in it.
func NewReplayCache(app framework.Application) (ReplayCache, error) {
	if app == nil {
		return nil, errors.New("jwtassert.NewReplayCache: app required")
	}
	core := app.AppCore()
	if core == nil {
		return nil, errors.New("jwtassert.NewReplayCache: app has no core")
	}
	switch core.CoordMode() {
	case coord.InProc:
		return NewReplayCacheInMemMap(), nil
	case coord.CrossProc:
		if core.MainKVDB == nil {
			return nil, errors.New("jwtassert.NewReplayCache: main kvdb not ready — under CrossProc the replay cache lives in it")
		}
		return NewReplayCacheKVDB(core.MainKVDB, core.AppName()+":ar:")
	default:
		return nil, fmt.Errorf("jwtassert.NewReplayCache: unknown coordination mode %v", core.CoordMode())
	}
}
