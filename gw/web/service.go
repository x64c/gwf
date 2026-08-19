package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	name       string             // registered instance identity; see NewServiceAs
	addr       string             // preserved across cycles for *http.Server rebuild
	handler    http.Handler       // preserved across cycles
	conf       ServerConf         // the server recipe (deadlines + drain window), validated by the loader
	ctx        context.Context    // per-cycle runtime context (set in Start)
	cancel     context.CancelFunc // per-cycle cancel (set in Start)
	reqCtx     context.Context    // per-cycle; parent of every request ctx — root's values, root's cancellation severed
	cancelReq  context.CancelFunc // fires when the drain window closes (see run)
	state      svc.AtomicState    // internal service state (State() may be read concurrently with lifecycle writes)
	terminated chan error         // one-shot; fires when Terminate completes
	stopped    chan struct{}      // per-cycle; closed when run goroutine has stopped
	listener   net.Listener       // per-cycle; bound by Start, so a bind failure fails the start
	server     *http.Server       // rebuilt each Start cycle (one-shot after Shutdown); unexported — a holder could Shutdown it or swap its handler behind the admission gate
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

func NewService(httpHandler http.Handler, conf ServerConf) *Service {
	return NewServiceAs("WebService", httpHandler, conf)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, httpHandler http.Handler, conf ServerConf) *Service {
	s := &Service{
		name:       name,
		addr:       conf.Listen,
		handler:    httpHandler,
		conf:       conf,
		terminated: make(chan error, 1),
	}
	s.state.Store(svc.StateREADY)
	return s
}

// Start : READY → RUNNING. Builds a fresh *http.Server (the previous one is
// dead after Shutdown), BINDS THE PORT, and hands the listener to run.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
//
// The bind happens HERE, and that is the whole point of this ordering. It used
// to happen inside the run goroutine, so Start set RUNNING, logged "listening
// on …", and returned nil before anything was bound — a port conflict then
// surfaced as an asynchronous [ERROR] while the service reported RUNNING for
// the life of the process with nothing listening, and the boot was "successful".
// Nothing may claim to have started until the step that can fail has succeeded.
//
// Atomic on failure: the contexts created above are canceled before returning,
// so a failed start leaves nothing behind for anyone to clean up.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	// Request contexts inherit the root's VALUES but not its cancellation:
	// canceling the root opens the drain, and canceling in-flight work at
	// that same moment would defeat it. They are canceled when the drain
	// window closes instead — see run.
	s.reqCtx, s.cancelReq = context.WithCancel(context.WithoutCancel(parentCtx))
	s.server = s.conf.newHTTPServer(s.addr, s.handler, s.reqCtx) // fresh per cycle

	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		s.cancel()
		s.cancelReq()
		return fmt.Errorf("listen(%q) failed: %w", s.addr, err)
	}
	s.listener = ln                 // fresh per cycle
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (listening on %s)", s.Name(), s.addr)
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit.
// The internal graceful drain uses the conf's drain window; ctx governs only
// how long Stop waits for the run goroutine to actually exit.
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
// if STOPPING, just wait for run goroutine to exit. Fires Terminated on EVERY
// exit path (deferred send): a missed stop deadline must still report, or
// WaitServicesTerminated counts N-1 forever and main wedges until SIGKILL.
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
	// LIFO: cancelReq first — the drain below has closed by the time run
	// returns, so a handler still in flight is canceled and can unwind
	// (release the connection, roll the transaction back) instead of being
	// hard-killed at process exit. Then the state transition, then stopped.
	defer close(s.stopped)
	defer s.transitionAfterRun()
	defer s.cancelReq()

	serverErr := make(chan error, 1)
	go func() {
		// Serve, not ListenAndServe: the listener was bound by Start, so by the
		// time this runs the only failures left are serving failures.
		if err := s.server.Serve(s.listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		} else {
			serverErr <- nil
		}
	}()

	select {
	case <-s.ctx.Done():
		s.server.SetKeepAlivesEnabled(false)
		gracefulCtx, cancel := context.WithTimeout(context.Background(), s.conf.drainTimeout())
		defer cancel()
		if err := s.server.Shutdown(gracefulCtx); err != nil {
			log.Printf("[ERROR][HTTPServer] shutdown failed: %v", err)
		}
		<-serverErr // wait for the Serve goroutine to return
	case err := <-serverErr:
		// server died on its own (serving error; a port conflict would have failed Start)
		if err != nil {
			log.Printf("[ERROR][HTTPServer] %v", err)
		}
	}
}

func (s *Service) transitionAfterRun() {
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
}
