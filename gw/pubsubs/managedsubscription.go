package pubsubs

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// managedSubscription is the part of a subscription its manager holds. It never points at the Subscription the
// subscriber holds, so a Subscription no code holds any more can be found unreachable even while this part is kept.
type managedSubscription struct {
	topics     []string
	queue      chan *Message
	endNotice  chan struct{} // closed once, when the subscription ends
	missNotice chan struct{} // capacity 1

	endReason atomic.Int64
	ended     atomic.Bool // set before endNotice closes; a put skips an ended subscription
	endOnce   sync.Once

	cleanup          runtime.Cleanup             // the abandonment watch; set before any other code can reach this part
	stopContextWatch atomic.Pointer[func() bool] // from context.AfterFunc; stored once Subscribe has registered it
}
