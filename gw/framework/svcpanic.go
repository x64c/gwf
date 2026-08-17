package framework

import "fmt"

// svcPanicError carries a panic recovered from a service's lifecycle call
// across the walk's goroutine boundary, as an error through the slots the walk
// already collects. The stack is captured where the panic happened — the only
// place it exists — and is logged there; val is re-raised as-is on the
// caller's goroutine by WaitServicesTerminated.
type svcPanicError struct {
	node  string
	val   any
	stack []byte
}

func (e *svcPanicError) Error() string {
	return fmt.Sprintf("service %q panicked: %v", e.node, e.val)
}
