package web

import "context"

// drainNoticeKey is the private context key under which a serving Service stores its drain notice.
type drainNoticeKey struct{}

// DrainNoticeReceiverFromContext returns the receive end of the drain notice of the web Service serving this request:
// a channel closed when that Service starts its drain. ok is false outside a request it serves.
//
// The notice cancels nothing. An ordinary handler ignores it and finishes within the drain window as before. A
// long-lived handler (a stream) selects on it to finish now, instead of running until its request context is canceled
// when the drain window closes.
func DrainNoticeReceiverFromContext(ctx context.Context) (<-chan struct{}, bool) {
	ch, ok := ctx.Value(drainNoticeKey{}).(<-chan struct{})
	return ch, ok
}
