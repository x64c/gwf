package session

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/web/session/bearer"
	"github.com/x64c/gwf/gw/web/session/cookie"
	"github.com/x64c/gwf/gw/web/session/lockstore"
)

// Service is the framework's session-management service. It owns the shared
// session-lock storage and holds optional per-flavor session managers (bearer,
// cookie). Each manager is set by its own Prepare* method at boot; nil means
// the app doesn't use that flavor.
//
// Service runs a background cleanup goroutine that removes stale lock entries
// from SessionLocks. An entry is considered stale when:
//   - lastTouched is older than cleanupOlderThan (i.e., no recent Acquire), AND
//   - its corresponding KVDB key no longer exists (TTL-expired list).
//
// The age filter keeps the cleanup work proportional to the count of likely-
// stale entries, not the total entry count.
type Service struct {
	name             string             // registered instance identity; see NewServiceAs
	Ctx              context.Context    // per-cycle runtime context (set in Start)
	cancel           context.CancelFunc // per-cycle cancel (set in Start)
	state            svc.AtomicState    // internal service state (read on the request path by the session middleware)
	terminated       chan error         // one-shot; fires when Terminate completes
	stopped          chan struct{}      // per-cycle; closed when run goroutine has stopped
	cleanupCycle     time.Duration
	cleanupOlderThan time.Duration

	KVDB         kvdbs.DB
	SessionLocks *lockstore.Store // shared with managers via pointer

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

func NewService(kvdb kvdbs.DB, cleanupCycle time.Duration, cleanupOlderThan time.Duration) *Service {
	return NewServiceAs("SessionService", kvdb, cleanupCycle, cleanupOlderThan)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, kvdb kvdbs.DB, cleanupCycle time.Duration, cleanupOlderThan time.Duration) *Service {
	s := &Service{
		name:             name,
		terminated:       make(chan error, 1),
		cleanupCycle:     cleanupCycle,
		cleanupOlderThan: cleanupOlderThan,
		KVDB:             kvdb,
		SessionLocks:     lockstore.New(),
	}
	s.state.Store(svc.StateREADY)
	return s
}

// Serving reports whether a protocol should serve a request right now: the
// service is RUNNING and the protocol is enabled. The composition lives here
// because only the service sees both the lifecycle state and the protocol's
// switch; the protocol itself only knows its own Enabled().
//
// protocolMgr is the protocol's manager (a svc.Switchable). A nil protocolMgr
// — e.g. an unwired manager passed directly — reports false (not serving), so
// the caller can pass its manager field without a separate nil check.
func (s *Service) Serving(protocolMgr svc.Switchable) bool {
	if protocolMgr == nil {
		return false
	}
	return s.state.Load() == svc.StateRUNNING && protocolMgr.Enabled()
}

// Start : READY → RUNNING. parentCtx is the runtime cancellation lineage.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	s.Ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (cleanup cycle=%v exp=%v)", s.Name(), s.cleanupCycle, s.cleanupOlderThan)
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit.
func (s *Service) Stop(ctx context.Context) error {
	if s.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", s.state.Load())
	}
	s.state.Store(svc.StateSTOPPING)
	return s.stop(ctx)
}

// Terminate : any → TERMINATING (irreversible). If RUNNING, full stop;
// if STOPPING, just wait for run goroutine to exit. Fires Terminated.
func (s *Service) Terminate(ctx context.Context) (err error) {
	if s.state.Load() == svc.StateTERMINATING {
		return nil // idempotent — returns before the defer arms
	}
	prevState := s.state.Load()
	s.state.Store(svc.StateTERMINATING)
	log.Printf("[INFO][%s] Terminating.", s.Name())
	defer func() {
		s.terminated <- err // THE ONLY send site; unconditional, exactly once
		if err == nil {
			log.Printf("[INFO][%s] Terminated.", s.Name())
		} else {
			log.Printf("[ERROR][%s] Terminated with stop error: %v", s.Name(), err)
		}
	}()
	switch prevState {
	case svc.StateRUNNING:
		err = s.stop(ctx)
	case svc.StateSTOPPING:
		err = s.waitStopped(ctx)
	}
	return err
}

// stop runs the full stop activity: log "Stopping.", cancel, waitStopped.
func (s *Service) stop(ctx context.Context) error {
	log.Printf("[INFO][%s] Stopping.", s.Name())
	s.cancel()
	return s.waitStopped(ctx)
}

// waitStopped waits for the run goroutine to exit; logs "Stopped." on success.
func (s *Service) waitStopped(ctx context.Context) error {
	select {
	case <-s.stopped:
		log.Printf("[INFO][%s] Stopped.", s.Name())
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop deadline exceeded: %w", ctx.Err())
	}
}

func (s *Service) Terminated() <-chan error {
	return s.terminated
}

func (s *Service) run() {
	ticker := time.NewTicker(s.cleanupCycle)
	defer ticker.Stop()
	defer close(s.stopped)
	defer s.transitionAfterRun() // LIFO: runs first, before close(s.stopped)
	for {
		select {
		case <-s.Ctx.Done():
			return
		case now := <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[PANIC] recovered in session-locks cleanup: %v", r)
					}
				}()
				log.Printf("[INFO][%s] %v cleanup cycle ...", s.Name(), s.cleanupCycle)
				s.Cleanup(now)
			}()
		}
	}
}

func (s *Service) transitionAfterRun() {
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
}

// Cleanup walks SessionLocks, skips recently-touched entries (cheap), and
// deletes entries whose KVDB key no longer exists (the actual KVDB call is
// only made for entries idle ≥ cleanupOlderThan).
func (s *Service) Cleanup(now time.Time) {
	threshold := now.Add(-s.cleanupOlderThan).UnixNano()
	s.SessionLocks.Range(func(key string, entry *lockstore.LockEntry) bool {
		if entry.LastTouchedNano() > threshold {
			return true // recently active; skip cheap
		}
		exists, err := s.KVDB.Exists(s.Ctx, key)
		if err != nil {
			return true // transient KVDB error; skip this entry this cycle
		}
		if !exists {
			s.SessionLocks.Delete(key)
		}
		return true
	})
}
