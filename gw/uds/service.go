package uds

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net"
	"os"
	"os/user"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/x64c/gwf/gw/svc"
)

type Service struct {
	Conf Conf

	// cmdStore is deliberately NOT embedded-exported: the command registry's
	// maps are plain (written at construction, read by every connection
	// goroutine), so an exported mutation path would be a data race as well as
	// an escape hatch (svc.Service: no escape hatches). The store is complete
	// when the service is constructed.
	cmdStore *CommandStore

	name       string             // registered instance identity; see NewServiceAs
	ctx        context.Context    // per-cycle runtime context (set in Start)
	cancel     context.CancelFunc // per-cycle cancel (set in Start)
	state      svc.AtomicState    // internal service state (State() may be read concurrently with lifecycle writes)
	terminated chan error         // one-shot; fires when Terminate completes
	stopped    chan struct{}      // per-cycle; closed when run goroutine has stopped
	listener   net.Listener       // rebuilt each Start cycle
}

func (s *Service) Name() string {
	return s.name
}

func (s *Service) State() svc.State {
	return s.state.Load()
}

func NewService(conf Conf, cmdStore *CommandStore) *Service {
	return NewServiceAs("UDSService", conf, cmdStore)
}

// NewServiceAs is NewService with the name given explicitly. A name identifies
// a registered INSTANCE, not a type: it is what logs, status output and
// dependency declarations all refer to, and registration rejects a duplicate.
// The string is taken raw — uniqueness and legibility are the caller's.
func NewServiceAs(name string, conf Conf, cmdStore *CommandStore) *Service {
	s := &Service{
		name:       name,
		terminated: make(chan error, 1),
		Conf:       conf,
		cmdStore:   cmdStore,
	}
	s.state.Store(svc.StateREADY)
	return s
}

// Start : READY → RUNNING. parentCtx is the runtime cancellation lineage.
// Bootstrapping errors (listen/chmod failures) are returned immediately.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (s *Service) Start(parentCtx context.Context) error {
	if s.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if s.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", s.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", s.Name())
	// A file already at the socket path is an abnormality — a live incumbent,
	// an impostor, or a cleanup hole — and removing it would paper over
	// whichever it is (or unlink a running instance's socket). Diagnose and
	// refuse; the cure is proper cleanup by whoever owns the abnormality.
	if err := refuseOccupiedPath(s.Conf.SocketPath); err != nil {
		return err
	}
	// The socket must never exist at any mode other than the stated one, so
	// it is BORN at socket_mode via umask (bind creates the file at
	// 0777 &^ umask) rather than chmodded into shape after. The flip is
	// process-wide for the microseconds of the bind; anything created
	// concurrently gets tighter bits, never looser. The chmod below then
	// guarantees the exact final mode regardless of platform umask behavior.
	oldMask := syscall.Umask(int(0o777 &^ s.Conf.Mode().Perm()))
	listener, err := net.Listen("unix", s.Conf.SocketPath)
	syscall.Umask(oldMask)
	if err != nil {
		return fmt.Errorf("listen(%q) failed: %v", s.Conf.SocketPath, err)
	}
	if err = os.Chmod(s.Conf.SocketPath, s.Conf.Mode()); err != nil {
		_ = listener.Close()
		_ = os.Remove(s.Conf.SocketPath)
		return fmt.Errorf("chmod(%q) failed: %w", s.Conf.SocketPath, err)
	}
	s.listener = listener
	s.ctx, s.cancel = context.WithCancel(parentCtx)
	s.stopped = make(chan struct{}) // fresh per cycle
	s.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (listening on %q)", s.Name(), s.Conf.SocketPath)
	go s.run()
	return nil
}

// occupiedProbeTimeout bounds the liveness probe against an occupied socket
// path. A unix-socket connect is kernel-local and answers immediately in both
// directions (accepted, or ECONNREFUSED from a dead file); the bound exists
// only for the pathological case of a live listener with a full backlog,
// which must diagnose as "live" rather than hang the boot.
//
// Deliberately a constant, not conf: the value is consulted only in that
// pathology, and tuning it changes neither the diagnosis nor its correctness
// in any scenario — only how long an already-doomed boot takes to say so.
// Healthy cases never wait at all, so no deployment loses anything practical
// to this being hardcoded; a conf field here would be a knob connected to
// nothing.
const occupiedProbeTimeout = time.Second

// refuseOccupiedPath diagnoses whatever sits at path and refuses it by name.
// It never removes anything: cleanup is proper on every stop path, so a file
// here means something is wrong, and deleting it would destroy the evidence —
// or a running instance's socket.
func refuseOccupiedPath(path string) error {
	fi, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		// the normal case: the path is free
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot start: stat(%q): %w", path, err)
	}
	if fi.Mode()&fs.ModeSocket == 0 {
		return fmt.Errorf("cannot start: %q is occupied by a foreign non-socket file (%v) — investigate how it got there, then clean up", path, fi.Mode())
	}
	conn, err := net.DialTimeout("unix", path, occupiedProbeTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("cannot start: a live process is serving %q — another instance of this app, or an impostor; investigate before touching it", path)
	}
	return fmt.Errorf("cannot start: dead socket file at %q (unclean shutdown?) — remove it and start again", path)
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

// run - internal run loop
func (s *Service) run() {
	defer close(s.stopped)
	defer s.transitionAfterRun() // LIFO: runs first, before close(s.stopped)

	// goroutine to clean up when context is done
	go func() {
		<-s.ctx.Done()
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
		go s.handleConn(conn)
	}
}

func (s *Service) transitionAfterRun() {
	if s.state.Load() == svc.StateSTOPPING {
		s.state.Store(svc.StateREADY)
	}
}

// peerCreds is the connecting process's kernel-reported identity — uid
// (resolved to a username when the system knows one), gid, pid — via
// SO_PEERCRED. This is attribution for the audit log, read from the kernel
// and unfakeable by the client; it is NOT authorization, so an unreadable
// credential degrades to "peer=?" rather than refusing the connection.
func peerCreds(c net.Conn) string {
	uc, ok := c.(*net.UnixConn)
	if !ok {
		return "peer=?"
	}
	raw, err := uc.SyscallConn()
	if err != nil {
		return "peer=?"
	}
	var cred *syscall.Ucred
	var credErr error
	if err = raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil || credErr != nil {
		return "peer=?"
	}
	who := strconv.Itoa(int(cred.Uid))
	if u, lookErr := user.LookupId(who); lookErr == nil {
		who += "(" + u.Username + ")"
	}
	return fmt.Sprintf("uid=%s gid=%d pid=%d", who, cred.Gid, cred.Pid)
}

func (s *Service) handleConn(c net.Conn) {
	peer := peerCreds(c)
	log.Printf("[INFO][%s] client connected [%s]", s.Name(), peer)

	go func() {
		<-s.ctx.Done()
		_ = c.Close()
	}()

	defer func() {
		log.Printf("[INFO][%s] client connection closed [%s]", s.Name(), peer)
		if err := c.Close(); err != nil {
			if !errors.Is(err, net.ErrClosed) { // && !strings.Contains(err.Error(), "use of closed network connection")
				log.Printf("[ERROR][%s] closing client connection: %v\n", s.Name(), err)
			}
		}
	}()

	// Per-line cap: an over-cap line is ErrTooLong below, answered and closed.
	scanner := bufio.NewScanner(c)
	scanner.Buffer(nil, s.Conf.MaxLineBytes)

	for {
		_, _ = fmt.Fprint(c, "> ")
		if !scanner.Scan() {
			switch err := scanner.Err(); {
			case err == nil:
				log.Printf("[INFO][%s] client disconnected [%s]", s.Name(), peer)
			case errors.Is(err, bufio.ErrTooLong):
				_, _ = fmt.Fprintf(c, "ERROR> command line exceeds max_line_bytes (%d)\n", s.Conf.MaxLineBytes)
				log.Printf("[ERROR][%s] client line exceeded max_line_bytes (%d)", s.Name(), s.Conf.MaxLineBytes)
			default:
				log.Printf("[ERROR][%s] client connection read error: %v\n", s.Name(), err)
			}
			return
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		args := strings.Fields(line)
		cmdStr := args[0]
		if cmdStr == "q" || cmdStr == "quit" || cmdStr == "exit" {
			return
		}
		if cmdStr == "h" || cmdStr == "help" {
			s.cmdStore.PrintHelp(c)
			continue
		}
		// look it up in the command map
		if handler, ok := s.cmdStore.GetHandler(cmdStr); ok {
			log.Printf("[INFO][%s] [%s] `%s`\n", s.Name(), peer, line)
			_, _ = fmt.Fprintln(c)
			panicked, err := s.runHandler(handler, args[1:], c, peer, line)
			if panicked {
				// The connection dies with the answer below; the socket
				// serves on. Without this recover, one panicking handler
				// killed the whole process from this goroutine.
				_, _ = fmt.Fprintf(c, "ERROR> %v\n", err)
				return
			}
			if err != nil {
				_, _ = fmt.Fprintf(c, "ERROR> %v\n", err)
				log.Printf("[ERROR][%s] [%s] `%s` terminated: %v\n", s.Name(), peer, line, err)
			} else {
				log.Printf("[INFO][%s] [%s] `%s` completed\n", s.Name(), peer, line)
			}
			_, _ = fmt.Fprintln(c)
		} else {
			_, _ = fmt.Fprintf(c, "unknown command: %s\n\n", cmdStr)
		}
	}

}

// runHandler runs one command handler, converting a panic into an error and
// reporting that it panicked. The stack is logged here, where it exists.
func (s *Service) runHandler(h CommandHandler, args []string, c net.Conn, peer, line string) (panicked bool, err error) {
	defer func() {
		if rcv := recover(); rcv != nil {
			log.Printf("[PANIC][%s] [%s] `%s` handler panicked: %v\n%s", s.Name(), peer, line, rcv, debug.Stack())
			panicked = true
			err = fmt.Errorf("handler panicked: %v", rcv)
		}
	}()
	return false, h.HandleCommand(args, c)
}
