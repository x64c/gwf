package framework

// ServiceHandle is a gated reference to a service, and the only way a consumer
// is meant to reach one. The typed pointer is unreachable until Get reports the
// service admitted, so "used a service whose lifetime guarantee no longer
// holds" becomes a compile-shaped problem rather than a discipline one.
//
// A ServiceHandle is two words, copied by value, and holds no lock. Resolution
// happens once, when the handle is created; the per-use question is answered
// by one atomic load. Nothing consults Core, traverses the graph, or hashes a
// name on the path a request takes.
type ServiceHandle[T any] struct {
	node *ServiceNode
	svc  T
}

// Get returns the service if it may be used right now. The branch is taken in
// essentially every call, so it predicts perfectly; the load is a plain aligned
// read on the platforms we target.
func (h ServiceHandle[T]) Get() (T, bool) {
	if !h.node.admitted.Load() {
		var zero T
		return zero, false
	}
	return h.svc, true
}

// Node reports the node this handle gates. For status reporting and logs —
// reading the service through it would defeat the gate.
func (h ServiceHandle[T]) Node() *ServiceNode { return h.node }

// newServiceHandle binds a typed pointer to the node that gates it.
func newServiceHandle[T any](n *ServiceNode, s T) ServiceHandle[T] {
	return ServiceHandle[T]{node: n, svc: s}
}

// absentServiceHandle is the handle for an optional dependency the app did not
// wire. It reports unavailable forever, so a consumer's no-dependency path is
// the same code as its dependency-unavailable path.
func absentServiceHandle[T any]() ServiceHandle[T] {
	return ServiceHandle[T]{node: neverAdmitted}
}
