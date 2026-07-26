package fwupstream

import (
	"io"
	"net/http"
)

// RequestPayload bundles caller-provided inputs to a request: extra headers
// and a body source. Body is supplied via a closure so the framework can
// rebuild a fresh reader on stdlib replays (HTTP redirects, HTTP/2 retries)
// and on framework-level retries driven by the session-scope caller.
type RequestPayload struct {
	Headers      http.Header               // extra headers; framework auth headers (Client-Id, Authorization) are written last and override conflicts
	BodyProvider func() (io.Reader, error) // produces a fresh body reader on each call; nil for body-less requests
}
