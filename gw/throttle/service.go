package throttle

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	name             string             // registered instance identity; see NewServiceAs
	ctx              context.Context    // per-cycle runtime context (set in Start)
	cancel           context.CancelFunc // per-cycle cancel (set in Start)
	state            svc.AtomicState    // internal service state (read on the request path via Allow)
	terminated       chan error         // one-shot; fires when Terminate completes
	stopped          chan struct{}      // per-cycle; closed when run goroutine has stopped
	cleanupCycle     time.Duration
	cleanupOlderThan time.Duration
	groups           map[string]*BucketGroup // groupID -> *BucketGroup
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

func NewService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) *Service {
	return NewServiceAs("ThrottleService", cleanupCycle, cleanupOlderThan)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, cleanupCycle time.Duration, cleanupOlderThan time.Duration) *Service {
	s := &Service{
		name:             name,
		terminated:       make(chan error, 1),
		cleanupCycle:     cleanupCycle,
		cleanupOlderThan: cleanupOlderThan,
		groups:           make(map[string]*BucketGroup),
	}
	s.state.Store(svc.StateREADY)
	return s
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
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (cleanup cycle=%v exp=%v)", s.Name(), s.cleanupCycle, s.cleanupOlderThan)
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit.
// ctx is the operation deadline.
func (s *Service) Stop(ctx context.Context) error {
	if s.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", s.state.Load())
	}
	s.state.Store(svc.StateSTOPPING)
	return s.stop(ctx)
	// transitionAfterRun in run goroutine flips STOPPING → READY
}

// Terminate : any → TERMINATING (irreversible). If currently RUNNING, runs
// the full stop activity. If STOPPING (Stop already canceled, possibly
// timed out), just waits for the run goroutine to actually exit. Fires
// Terminated when complete.
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
// Called by Stop and by Terminate (when prior state was RUNNING).
func (s *Service) stop(ctx context.Context) error {
	log.Printf("[INFO][%s] Stopping.", s.Name())
	s.cancel()
	return s.waitStopped(ctx)
}

// waitStopped waits for the run goroutine to exit; logs "Stopped." on success.
// Called by stop() and by Terminate (when prior state was STOPPING — cancel
// was already issued by the in-flight Stop, which may have ctx-timed out).
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
	defer close(s.stopped)       // declared 2nd → runs 2nd-to-last (after transitionAfterRun)
	defer s.transitionAfterRun() // declared 3rd → runs FIRST (LIFO defer order)
	for {
		select {
		case <-s.ctx.Done():
			return
		case now := <-ticker.C:
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[PANIC][%s] recovered in cleanup cycle: %v", s.Name(), r)
					}
				}()
				log.Printf("[INFO][%s] %v cleanup cycle ...", s.Name(), s.cleanupCycle)
				s.cleanup(now)
			}()
		}
	}
}

// transitionAfterRun moves the state out of STOPPING into READY once the run
// goroutine has exited. If state is TERMINATING, Terminate's flow handles the
// rest — we leave the state alone.
func (s *Service) transitionAfterRun() {
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
}

// getBucketGroup is internal on purpose: a *BucketGroup reaches live buckets,
// and a live bucket escaping through an exported accessor is callable behind
// the admission gate for the life of the process (svc.Service: no escape
// hatches). External callers get Allow (the verdict), HasGroup (the boot-time
// existence question) and Inspect (a snapshot).
func (s *Service) getBucketGroup(id string) (*BucketGroup, bool) {
	g, ok := s.groups[id]
	return g, ok
}

// HasGroup reports whether a bucket group is registered under id. Groups are
// boot wiring (see SetBucketGroup), so after Start the answer is stable — this
// is what lets a wrapper validate its group id at wrap time and turn a mistype
// into a named boot failure instead of a permanently dead route.
func (s *Service) HasGroup(id string) bool {
	_, ok := s.groups[id]
	return ok
}

// SetBucketGroup registers a bucket group by id. Groups are BOOT WIRING: the
// groups map is plain (no mutex) and is only race-free under the contract that
// all writes happen before Start, then only reads occur (request handlers +
// cleanup ticker). A call in any state but READY is refused with an error —
// refusal, not a process kill: the sequencing mistake is the caller's, and a
// boot-time caller keeps fail-fast at its own call site by treating the error
// as fatal there.
func (s *Service) SetBucketGroup(id string, conf *BucketConf) error {
	if state := s.state.Load(); state != svc.StateREADY {
		return fmt.Errorf("throttle %q: can't set bucket group %q: state is %v — groups are boot wiring, set before Start", s.Name(), id, state)
	}
	s.groups[id] = &BucketGroup{
		conf:    conf,
		buckets: &bucketMap{},
	}
	return nil
}

// Allow reports the bucket's verdict for one use of bucketID within groupID.
// An unknown groupID is always blocked.
//
// Allow answers the RATE question only. Whether this service may be used right
// now is not its to answer — reachability is decided in front of the pointer,
// by the framework (svc.Service; consumers reach the service through a
// framework handle). The buckets are passive state, so they survive Stop and
// keep computing honest verdicts; a bool cannot express "unavailable", which
// is exactly why no lifecycle answer may be folded into this one.
func (s *Service) Allow(groupID string, bucketID string, now time.Time) bool {
	g, ok := s.getBucketGroup(groupID)
	if !ok {
		return false // Invalid groupID -> always Blocked
	}
	return g.loadOrCreateBucket(bucketID, now).Allow(now)
}

// Inspect returns a snapshot of all BucketGroup IDs and their local Bucket IDs.
// It does not lock globally, so results may be slightly inconsistent
// if buckets are being modified concurrently — which is fine for inspection.
func (s *Service) Inspect() map[string][]string {
	result := make(map[string][]string)

	for groupID, bucketGroup := range s.groups {
		var ids []string
		bucketGroup.buckets.rangeAll(func(id string, _ *Bucket) bool {
			ids = append(ids, id)
			return true
		})
		result[groupID] = ids
	}

	return result
}
