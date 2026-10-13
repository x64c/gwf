package sse

import "net/http"

// RefuseStream answers a stream request with status and no stream. Any status but 200 makes a conforming client
// (EventSource) close for good instead of reconnecting; 204 is the one the standard mentions for that. 200 would read
// as the start of a stream, so it panics.
func RefuseStream(w http.ResponseWriter, status int) {
	if status == http.StatusOK {
		panic("sse: RefuseStream with 200 — a 200 starts a stream")
	}
	w.WriteHeader(status)
}
