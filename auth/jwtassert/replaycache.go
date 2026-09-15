package jwtassert

import (
	"context"
	"time"
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
