package fwupstream

// MachineSigner authenticates one request as the downstream (a machine
// client) to the upstream: it returns the Authorization header value for
// the request as described. body is nil when the request has none; extra carries
// further claims the assertion must name (the exchange's user claim).
//
// Implemented by a machine authentication method's client half; this
// package names none.
type MachineSigner interface {
	SignRequest(method, target string, body []byte, extra map[string]any) (authorization string, err error)
}
