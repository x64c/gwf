package sse

// Stats is a snapshot of the open streams, and of what has ended streams since the service was built.
type Stats struct {
	StreamCount            int // open now
	FrameWriteTimeoutCount int // streams ended because the client could not take a frame within frame_write_timeout_secs
}
