package sse

import "net/http"

// LastEventID returns the ID the client says it last received: the Last-Event-ID header when present, otherwise the
// query parameter named param ("" = the header only).
//
// The header comes first because a browser's own reconnect re-sends the URL the page opened the stream with — so a
// query parameter there is as old as that page's open — while the header always carries the latest ID. Page code
// that closes a stream and opens a new one has to pass the last ID itself, which is what the parameter is for.
func LastEventID(r *http.Request, param string) string {
	if id := r.Header.Get("Last-Event-ID"); id != "" {
		return id
	}
	if param == "" {
		return ""
	}
	return r.URL.Query().Get(param)
}
