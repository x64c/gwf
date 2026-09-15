package fwauthserver

import "fmt"

// UpstreamError is a non-200 answer from the auth server, carried whole so
// the caller can forward it.
type UpstreamError struct {
	StatusCode int
	Body       []byte
}

func (e *UpstreamError) Error() string { return fmt.Sprintf("auth server answered %d", e.StatusCode) }
