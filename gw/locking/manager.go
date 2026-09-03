package locking

import (
	"context"
	"sync"
)

// Manager is what callers speak to. It takes asks, runs the caller's function
// while the name is held, releases at return, and answers what its instance
// holds. Callers never touch a ledger.
//
// A name held anywhere, by this instance or another, refuses the ask at once
// with errs.ActionLocked, and asking again is the caller's decision.
//
// Every manager is built with a name of its own — what it serves, such as
// "action" or "session" — so that an app running several tells them apart in
// its logs and its admin surfaces, and a manager over a shared store can be
// given a region of its own from it.
type Manager interface {
	// Name is the manager's own name, given at construction.
	Name() string
	// AcquireDoRelease asks for name, runs fn while the name is held, and
	// releases the name when fn returns, by any path, a panic included. A
	// held name refuses the ask at once with errs.ActionLocked and fn does
	// not run; so does a ctx that has already ended, with its error. fn's
	// own error is returned as is. The ctx given to fn derives from the
	// caller's and ends when fn returns.
	AcquireDoRelease(ctx context.Context, name string, fn func(ctx context.Context) error) error
	// AcquireDoReleaseAll asks for every name or none, in the order given,
	// and releases them in reverse. One refusal releases what was already
	// taken and returns errs.ActionLocked naming the refused name.
	AcquireDoReleaseAll(ctx context.Context, names []string, fn func(ctx context.Context) error) error
	// Names reports, as a snapshot, the names this instance holds: the
	// internal ledger.
	Names() []string
}

// internalLedger is the record built into every manager: the names this
// instance holds right now, each with what its manager keeps beside it,
// under one mutex. A name is held exactly while it is a key here.
type internalLedger[V any] struct {
	mu   sync.Mutex
	held map[string]V
}

// reserve records name as held, with v beside it, unless it already is.
func (l *internalLedger[V]) reserve(name string, v V) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if _, held := l.held[name]; held {
		return false
	}
	l.held[name] = v
	return true
}

// release forgets name. Forgetting a name not held is a no-op.
func (l *internalLedger[V]) release(name string) {
	l.mu.Lock()
	delete(l.held, name)
	l.mu.Unlock()
}

// names is a snapshot of what is held.
func (l *internalLedger[V]) names() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	names := make([]string, 0, len(l.held))
	for name := range l.held {
		names = append(names, name)
	}
	return names
}

// runUnder runs fn under a ctx derived from ctx that ends when fn returns.
func runUnder(ctx context.Context, fn func(context.Context) error) error {
	fnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	return fn(fnCtx)
}
