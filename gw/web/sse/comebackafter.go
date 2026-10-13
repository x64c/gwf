package sse

import (
	"net/http"
	"strconv"
	"time"
)

// ComeBackAfter answers a stream request with a stream that ends at once, telling the client to come back after wait:
// a conforming client (EventSource) reconnects by itself then. wait is sent in whole milliseconds, rounded up; it
// panics when wait is not above 0, since a client would come straight back.
func ComeBackAfter(w http.ResponseWriter, wait time.Duration) {
	if wait <= 0 {
		panic("sse: ComeBackAfter with a wait not above 0")
	}
	millis := (wait + time.Millisecond - 1) / time.Millisecond
	setStreamHeaders(w)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("retry: " + strconv.FormatInt(int64(millis), 10) + "\n\n"))
}

func setStreamHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("X-Accel-Buffering", "no") // asks a buffering reverse proxy to pass each frame on at once
}
