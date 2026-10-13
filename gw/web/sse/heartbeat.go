package sse

import (
	"errors"
	"time"
)

// commentFrame is the heartbeat when no event is given: a comment line, which a client ignores and every proxy on the
// path counts as traffic.
var commentFrame = []byte(": ping\n\n")

// Heartbeat is what a stream sends when it has been quiet: after Silence without any write, one frame.
type Heartbeat struct {
	Silence time.Duration // REQUIRED (> 0): how long a stream may go without a write before a heartbeat is sent
	Event   *Event        // nil: a comment (keeps the path open; page code never sees it); set: this event (page code can watch for it)
}

func (h Heartbeat) validate() error {
	if h.Silence <= 0 {
		return errors.New("sse: heartbeat silence must be set (> 0)")
	}
	if h.Event != nil {
		return h.Event.validate()
	}
	return nil
}

// frame is the heartbeat's wire form.
func (h Heartbeat) frame() []byte {
	if h.Event == nil {
		return commentFrame
	}
	return h.Event.appendFrame(nil)
}
