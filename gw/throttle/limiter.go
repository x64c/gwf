package throttle

import (
	"context"
	"time"
)

// Limiter answers the rate question for one use of a bucket. Whether the
// limiter itself may be used is not its to answer: reachability is decided in
// front of the pointer, by the framework handle a consumer holds it through.
//
// Allow's error is not a verdict. It reports that NO verdict could be computed
// — what a limiter counting outside this process has to say while the store
// holding its counters is unreachable. A limiter counting in its own memory
// has no such failure and always returns a nil error. What an unanswered
// request is given is the caller's to decide, and to state.
//
// HasGroup is asked while routes are built, not per request, so a bucket group
// id that names nothing can be a boot failure rather than a route that refuses
// every request for the life of the process.
type Limiter interface {
	Allow(ctx context.Context, groupID string, bucketID string, now time.Time) (bool, error)
	HasGroup(groupID string) bool
}

var _ Limiter = (*Service)(nil)
