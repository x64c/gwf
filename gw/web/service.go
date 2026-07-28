package web

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	addr         string             // preserved across cycles for *http.Server rebuild
	handler      http.Handler       // preserved across cycles
	drainTimeout time.Duration      // HTTP graceful-drain window on shutdown (Server.Shutdown budget); set at construction
	Ctx          context.Context    // per-cycle runtime context (set in Start)
	cancel       context.CancelFunc // per-cycle cancel (set in Start)
	state        svc.State          // internal service state
	terminated   chan error         // one-shot; fires when Terminate completes
	stopped      chan struct{}      // per-cycle; closed when run goroutine has stopped
	Server       *http.Server       // rebuilt each Start cycle (one-shot after Shutdown)
}

func (s *Service) Name() string {
	return "WebService"
}

func (s *Service) State() svc.State {
	return s.state
}

func NewService(addr string, httpHandler http.Handler, drainTimeout time.Duration) *Service {
	return &Service{
		addr:         addr,
		handler:      httpHandler,
		drainTimeout: drainTimeout,
		state:        svc.StateREADY,
		terminated:   make(chan error, 1),
	}
}

// Start : READY → RUNNING. Builds a fresh *http.Server (the previous one is
// dead after Shutdown), binds the port, and serves.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state)
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	s.Server = &http.Server{Addr: s.addr, Handler: s.handler} // fresh per cycle
	s.Ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state = svc.StateRUNNING
	log.Printf("[INFO][%s] Running. (listening on %s)", s.Name(), s.addr)
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit.
// The internal graceful drain uses the drainTimeout set at construction; ctx
// governs only how long Stop waits for the run goroutine to actually exit.
func (s *Service) Stop(ctx context.Context) error {
	if s.state == svc.StateREADY {
		return nil // idempotent
	}
	if s.state != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", s.state)
	}
	s.state = svc.StateSTOPPING
	return s.stop(ctx)
}

// Terminate : any → TERMINATING (irreversible). If RUNNING, full stop;
// if STOPPING, just wait for run goroutine to exit. Fires Terminated on EVERY
// exit path (deferred send): a missed stop deadline must still report, or
// WaitServicesTerminated counts N-1 forever and main wedges until SIGKILL.
func (s *Service) Terminate(ctx context.Context) (err error) {
	if s.state == svc.StateTERMINATING {
		return nil // idempotent — returns before the defer arms
	}
	prevState := s.state
	s.state = svc.StateTERMINATING
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
	defer close(s.stopped)
	defer s.transitionAfterRun() // LIFO: runs first, before close(s.stopped)

	serverErr := make(chan error, 1)
	go func() {
		if err := s.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		} else {
			serverErr <- nil
		}
	}()

	select {
	case <-s.Ctx.Done():
		s.Server.SetKeepAlivesEnabled(false)
		gracefulCtx, cancel := context.WithTimeout(context.Background(), s.drainTimeout)
		defer cancel()
		if err := s.Server.Shutdown(gracefulCtx); err != nil {
			log.Printf("[ERROR][HTTPServer] shutdown failed: %v", err)
		}
		<-serverErr // wait for ListenAndServe goroutine to return
	case err := <-serverErr:
		// server died on its own (port conflict, internal error)
		if err != nil {
			log.Printf("[ERROR][HTTPServer] %v", err)
		}
	}
}

func (s *Service) transitionAfterRun() {
	if s.state == svc.StateSTOPPING {
		s.state = svc.StateREADY
	}
}
