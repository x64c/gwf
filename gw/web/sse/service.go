package sse

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/x64c/gwf/gw/svc"
)

// errNotRunning is what Serve returns after refusing a stream because the service is not running.
var errNotRunning = errors.New("sse: service not running")

// Service owns every open SSE stream in this process: their lifetimes, and how long each frame may take to write.
// Stopping it ends every stream it holds; while it is stopped, Serve refuses new ones (204, so clients do not come
// back). What a stream sends is never the service's: each Serve call brings its own Source.
type Service struct {
	name       string
	conf       Conf
	state      svc.AtomicState
	terminated chan error // one-shot; fires when Terminate completes

	mu          sync.Mutex         // guards everything below
	ctx         context.Context    // per-cycle (Start); canceled by Stop/Terminate, which ends every stream of the cycle
	cancel      context.CancelFunc // per-cycle
	stopped     chan struct{}      // per-cycle; closed once every stream of the cycle has ended
	open        sync.WaitGroup     // each open stream; Add only under mu while ctx is live
	streamCount int

	frameWriteTimeouts atomic.Int64
}

func NewService(conf Conf) *Service {
	return NewServiceAs("SSEService", conf)
}

// NewServiceAs is NewService with the name given explicitly; see web.NewServiceAs.
func NewServiceAs(name string, conf Conf) *Service {
	s := &Service{
		name:       name,
		conf:       conf,
		terminated: make(chan error, 1),
	}
	s.state.Store(svc.StateREADY)
	return s
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

// Start : READY → RUNNING. Opens a new cycle; nothing in it can fail.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	s.mu.Lock()
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{})
	ctx, stopped := s.ctx, s.stopped
	s.mu.Unlock()
	go s.run(ctx, stopped)
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (frame write timeout %ds, retry %dms)", s.Name(), s.conf.FrameWriteTimeoutSecs, s.conf.RetryMillis)
	return nil
}

// Stop : RUNNING → STOPPING → READY. Ends every open stream (each is told to reconnect after retry_millis, and is
// refused when it does while the service stays stopped) and returns once all have ended. ctx governs only how long
// Stop waits.
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

// Terminate : any → TERMINATING (irreversible). If RUNNING, full stop; if STOPPING, just wait. Fires Terminated on
// EVERY exit path (deferred send).
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

func (s *Service) Terminated() <-chan error {
	return s.terminated
}

// stop cancels the cycle under mu — after which no stream can register — then waits for the cycle to end.
func (s *Service) stop(ctx context.Context) error {
	log.Printf("[INFO][%s] Stopping.", s.Name())
	s.mu.Lock()
	s.cancel()
	s.mu.Unlock()
	return s.waitStopped(ctx)
}

func (s *Service) waitStopped(ctx context.Context) error {
	s.mu.Lock()
	stopped := s.stopped
	s.mu.Unlock()
	select {
	case <-stopped:
		log.Printf("[INFO][%s] Stopped.", s.Name())
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop deadline exceeded: %w", ctx.Err())
	}
}

// run waits for the cycle to be canceled, then for every stream of the cycle to end.
func (s *Service) run(ctx context.Context, stopped chan struct{}) {
	<-ctx.Done()
	s.open.Wait()
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
	close(stopped)
}

// register counts a stream in if the cycle is live, under mu — the same lock stop cancels under, so once a cycle is
// canceled no stream can join it.
func (s *Service) register() (cycleDone <-chan struct{}, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil {
		return nil, false
	}
	s.streamCount++
	s.open.Add(1)
	return s.ctx.Done(), true
}

func (s *Service) unregister() {
	s.mu.Lock()
	s.streamCount--
	s.mu.Unlock()
	s.open.Done()
}

// write sends b and flushes it, with frame_write_timeout_secs to complete: a client that cannot take a frame within
// it is treated as gone, so no stream can hold a stop or a drain open.
func (s *Service) write(w http.ResponseWriter, rc *http.ResponseController, b []byte) error {
	if err := rc.SetWriteDeadline(time.Now().Add(s.conf.frameWriteTimeout())); err != nil {
		return err
	}
	if len(b) > 0 {
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	return rc.Flush()
}

// noteWriteError counts a write that did not finish within frame_write_timeout_secs: the client stopped taking
// frames. Any other write error is the client gone (a reset, a broken pipe) — the same event as its request context
// ending, which is not counted however it was noticed first.
func (s *Service) noteWriteError(err error) {
	if errors.Is(err, os.ErrDeadlineExceeded) {
		s.frameWriteTimeouts.Add(1)
	}
}

// endWithRetry tells the client when to reconnect; the stream is ending either way, so a failed write is ignored.
func (s *Service) endWithRetry(w http.ResponseWriter, rc *http.ResponseController) {
	_ = s.write(w, rc, []byte("retry: "+strconv.Itoa(s.conf.RetryMillis)+"\n\n"))
}

// Stats returns a snapshot of the open streams and the end counter.
func (s *Service) Stats() Stats {
	s.mu.Lock()
	n := s.streamCount
	s.mu.Unlock()
	return Stats{StreamCount: n, FrameWriteTimeoutCount: int(s.frameWriteTimeouts.Load())}
}
