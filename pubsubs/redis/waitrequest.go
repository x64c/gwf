package redis

// waitRequest is Subscribe asking the broker's loop to tell it once every one of topics is confirmed at Redis.
type waitRequest struct {
	topics    []string
	endNotice <-chan struct{} // the subscription's; once closed, Subscribe has returned and the loop drops the request
	reply     chan struct{}   // capacity 1, so the loop never waits to answer
}
