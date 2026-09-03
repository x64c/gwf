package session

import (
	"context"
	"fmt"
	"log"

	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/locking"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/web/session/bearer"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// Service is the framework's session-management service. It holds the locking
// manager the session managers share and the optional per-flavor session
// managers (bearer, cookie). Each manager is set by its own Prepare* method at
// boot; nil means the app doesn't use that flavor.
//
// Service runs nothing in the background: the locking manager keeps only what
// is held right now (see g/locking), so this service's lifecycle is the
// admission gate and only that.
type Service struct {
	name       string          // registered instance identity; see NewServiceAs
	state      svc.AtomicState // internal service state (read on the request path by the session middleware)
	terminated chan error      // one-shot; fires when Terminate completes

	KVDB           kvdbs.DB
	lockingManager locking.Manager // set at construction; shared with the managers, see LockingManager

	// Per-protocol managers. Each owns its own enable/disable toggle (it
	// satisfies Switchable) and is nil if the app doesn't wire that protocol.
	BearerSessionManager *bearer.SessionManager
	CookieSessionManager *cookie.SessionManager
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

// LockingManager is the locking manager this service was built with: the one
// every session manager under it holds its names on.
func (s *Service) LockingManager() locking.Manager {
	return s.lockingManager
}

func NewService(kvdb kvdbs.DB, lockingManager locking.Manager) *Service {
	return NewServiceAs("SessionService", kvdb, lockingManager)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, kvdb kvdbs.DB, lockingManager locking.Manager) *Service {
	s := &Service{
		name:           name,
		terminated:     make(chan error, 1),
		KVDB:           kvdb,
		lockingManager: lockingManager,
	}
	s.state.Store(svc.StateREADY)
	return s
}

// Start : READY → RUNNING. There is no background work to begin — Start opens
// the admission gate. parentCtx goes unread; the parameter is the svc.Service
// contract. Lifecycle methods (Start/Stop/Terminate) are not safe to call
// concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running.", s.Name())
	return nil
}

// Stop : RUNNING → READY. Nothing runs, so nothing is waited for; ctx goes
// unread and the transition is immediate.
func (s *Service) Stop(ctx context.Context) error {
	if s.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", s.state.Load())
	}
	s.state.Store(svc.StateREADY)
	log.Printf("[INFO][%s] Stopped.", s.Name())
	return nil
}

// Terminate : any → TERMINATING (irreversible). Nothing runs, so there is no
// stop activity to perform; Terminated fires immediately and always nil.
func (s *Service) Terminate(ctx context.Context) error {
	if s.state.Load() == svc.StateTERMINATING {
		return nil // idempotent — the terminated send happens exactly once
	}
	s.state.Store(svc.StateTERMINATING)
	s.terminated <- nil // THE ONLY send site; unconditional, exactly once
	log.Printf("[INFO][%s] Terminated.", s.Name())
	return nil
}

func (s *Service) Terminated() <-chan error {
	return s.terminated
}
