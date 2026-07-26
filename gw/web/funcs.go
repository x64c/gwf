package web

import "net/http"

// ShallowCloneClient returns a shallow copy of the given *http.Client.
// The clone shares the Transport and other pointer fields, but config fields like Timeout are independent.
// new(*base) allocates a new value initialized to a copy of *base (dereference), and returns a pointer to it.
// Equivalent to: c := *base; return &c
func ShallowCloneClient(base *http.Client) *http.Client {
	return new(*base)
}
