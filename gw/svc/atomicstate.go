package svc

import "sync/atomic"

// AtomicState is a concurrency-safe holder for State. Use it for a service's
// state field when State() may be read concurrently with the lifecycle writes
// from Start/Stop/Terminate — a plain State field would race in that case.
//
// The zero value is not a valid State (0 is none of the State consts);
// Store an initial state (typically StateREADY) at construction.
type AtomicState struct {
	v atomic.Int32
}

func (a *AtomicState) Load() State   { return State(a.v.Load()) }
func (a *AtomicState) Store(s State) { a.v.Store(int32(s)) }
