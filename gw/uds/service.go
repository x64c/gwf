package uds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	*CommandStore // [Embedded]
	Conf          Conf
	Ctx           context.Context // per-cycle runtime context (set in Start)

	name       string             // registered instance identity; see NewServiceAs
	cancel     context.CancelFunc // per-cycle cancel (set in Start)
	state      svc.State          // internal service state
	terminated chan error         // one-shot; fires when Terminate completes
	stopped    chan struct{}      // per-cycle; closed when run goroutine has stopped
	listener   net.Listener       // rebuilt each Start cycle
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state
}

func NewService(conf Conf, cmdStore *CommandStore) *Service {
	return NewServiceAs("UDSService", conf, cmdStore)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, conf Conf, cmdStore *CommandStore) *Service {
	return &Service{
		name:         name,
		state:        svc.StateREADY,
		terminated:   make(chan error, 1),
		Conf:         conf,
		CommandStore: cmdStore,
	}
}

// Start : READY → RUNNING. parentCtx is the runtime cancellation lineage.
// Bootstrapping errors (listen/chmod failures) are returned immediately.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state)
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	// clean up old socket if any (previous cycle may have left it)
	_ = os.Remove(s.Conf.SocketPath)
	// create socket
	listener, err := net.Listen("unix", s.Conf.SocketPath)
	if err != nil {
		return fmt.Errorf("listen(%q) failed: %v", s.Conf.SocketPath, err)
	}
	// tighten permissions immediately after binding
	if err = os.Chmod(s.Conf.SocketPath, 0660); err != nil {
		_ = listener.Close()
		_ = os.Remove(s.Conf.SocketPath)
		return fmt.Errorf("chmod(%q) failed: %w", s.Conf.SocketPath, err)
	}
	s.listener = listener
	s.Ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state = svc.StateRUNNING
	log.Printf("[INFO][%s] Running. (listening on %q)", s.Name(), s.Conf.SocketPath)
	go s.run()
	return nil
}

// Stop : RUNNING → STOPPING → READY. Synchronous on the run goroutine's exit.
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
// if STOPPING, just wait for run goroutine to exit. Fires Terminated.
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

// run - internal run loop
func (s *Service) run() {
	defer close(s.stopped)
	defer s.transitionAfterRun() // LIFO: runs first, before close(s.stopped)

	// goroutine to clean up when context is done
	go func() {
		<-s.Ctx.Done()
		if err := s.listener.Close(); err != nil {
			log.Printf("[ERROR][%s] cannot close socket (listener): %v", s.Name(), err)
		}
		// To avoid TOCTOU race, just try removing before checking if it exists.
		if err := os.Remove(s.Conf.SocketPath); err != nil && !os.IsNotExist(err) {
			log.Printf("[ERROR][%s] cannot remove socket file: %v", s.Name(), err)
		}
	}()

	// --- Serving loop ---
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				log.Printf("[INFO][%s] socket (listener) closed", s.Name())
				return
			}
			// For transient errors, don't kill the loop
			log.Printf("[ERROR][%s] socket (listener) accept failed: %v", s.Name(), err)
			continue
		}
		log.Printf("[INFO][%s] client connected", s.Name())
		go s.handleConn(conn)
	}
}

func (s *Service) transitionAfterRun() {
	if s.state == svc.StateSTOPPING {
		s.state = svc.StateREADY
	}
}

func (s *Service) handleConn(c net.Conn) {
	go func() {
		<-s.Ctx.Done()
		_ = c.Close()
	}()

	defer func() {
		log.Printf("[INFO][%s] client connection closed", s.Name())
		if err := c.Close(); err != nil {
			if !errors.Is(err, net.ErrClosed) { // && !strings.Contains(err.Error(), "use of closed network connection")
				log.Printf("[ERROR][%s] closing client connection: %v\n", s.Name(), err)
			}
		}
	}()

	reader := bufio.NewReader(io.LimitReader(c, 1<<20)) // 1 MB max per line

	for {
		_, _ = fmt.Fprint(c, "> ")
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				log.Printf("[INFO][%s] client disconnected", s.Name())
			} else {
				log.Printf("[ERROR][%s] client connection read error: %v\n", s.Name(), err)
			}
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		args := strings.Fields(line)
		cmdStr := args[0]
		if cmdStr == "q" || cmdStr == "quit" || cmdStr == "exit" {
			return
		}
		if cmdStr == "h" || cmdStr == "help" {
			s.CommandStore.PrintHelp(c)
			continue
		}
		// look it up in the command map
		if handler, ok := s.GetHandler(cmdStr); ok {
			log.Printf("[INFO][%s] `%s`\n", s.Name(), line)
			_, _ = fmt.Fprintln(c)
			if err = handler.HandleCommand(args[1:], c); err != nil {
				_, _ = fmt.Fprintf(c, "ERROR> %v\n", err)
				log.Printf("[ERROR][%s] `%s` terminated: %v\n", s.Name(), line, err)
			} else {
				log.Printf("[INFO][%s] `%s` completed\n", s.Name(), line)
			}
			_, _ = fmt.Fprintln(c)
		} else {
			_, _ = fmt.Fprintf(c, "unknown command: %s\n\n", cmdStr)
		}
	}

}
