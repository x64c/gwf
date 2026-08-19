package svc

import "context"

// Service is a lifecycle-managed component of an app.
//
// A service has two parts, and the lifecycle only ever governs the first:
//
//   - the ACTIVE part — its goroutines: a cleanup ticker, a scheduler loop, a
//     listener;
//   - the PASSIVE part — its in-memory state and the methods others call on
//     it: bucket groups, session locks, a command store.
//
// Read the three transitions in those terms:
//
//	Start      begins or RESUMES the active part.
//	Stop       PAUSES it. The passive part survives untouched, which is
//	           exactly what makes Start able to resume.
//	Terminate  ends the service for good.
//
// STOP IS A PAUSE, NOT AN OFF SWITCH. A stopped service still holds everything
// it held, and every method on it still works: halting a cleanup ticker does
// not empty the map it was cleaning. The name is kept for familiarity, but a
// service that is "stopped" is one whose goroutines are idle, not one that has
// gone away.
//
// AND NO STATE MAKES A SERVICE UNREACHABLE. Callers hold pointers; nothing in
// the runtime revokes them, so Stop and Terminate cannot prevent a call. Two
// consequences an implementation must honour:
//
//   - reachability is not the service's to control. Whether a caller may use
//     it is decided in front of the pointer, by the framework, which can tell
//     a caller "unavailable" in terms that caller understands. A method that
//     answers that question itself is inventing a second, divergent verdict —
//     and if its return type cannot express "unavailable", it will get it
//     wrong (a rate limiter returning "allowed" while stopped is the canonical
//     case).
//   - a method must therefore stay SAFE to call after Stop or Terminate:
//     returning stale results is acceptable, panicking or blocking forever is
//     not. The framework can stop new callers; only the service can survive
//     the ones already inside.
//
// It follows that a service must not hand out anything callable that bypasses
// this — an internal pointer escaping through an exported accessor is a door
// with no gate on it. Return a snapshot, or return something that consults the
// same authority the framework does.
//
// PANICS. A panic escaping Start, Stop or Terminate is recovered by the
// framework's lifecycle machinery, which completes what it owes (ordered
// rollback or teardown) and raises the panic again for the application. A
// panic in a goroutine the service spawns is different: nothing outside the
// service can recover it, and it ends the process. To keep such a failure
// inside the service's boundary, recover at the goroutine's top and report it
// as the service's failure — the error from Start, or the error on the
// Terminated send.
type Service interface {
	Name() string
	State() State

	// Start transitions READY → RUNNING, whether that is the first start or a
	// resume after Stop — the transition is the same either way. parentCtx is
	// the runtime cancellation lineage (typically Core.RootCtx); the service
	// derives its own per-cycle context from it.
	//
	// HONEST: return only once the service has genuinely started. Everything
	// that can fail — binding a port, opening a socket, acquiring a lease —
	// must be done and checked BEFORE returning nil. Reporting success and
	// then failing asynchronously makes every downstream guarantee a lie: the
	// framework marks the service RUNNING and admitted, its dependents start
	// against it, and the failure surfaces only as a log line nobody is
	// waiting for.
	//
	// ATOMIC: a failed start leaves nothing behind. Release whatever was
	// acquired before the failing step and return an error. The framework
	// rolls back the services that DID start; it cannot clean up inside one
	// that half-started, and cannot tell that case apart from a clean failure.
	//
	// (A future addition, tied to starting siblings concurrently: an operation
	// context so an in-flight Start can be canceled when a sibling fails,
	// instead of being waited for only to be torn down.)
	Start(parentCtx context.Context) error

	// Stop PAUSES the service: RUNNING → STOPPING (immediate) → READY (when the
	// run goroutine has exited). The passive state is preserved and the service
	// stays callable — see the type doc. Synchronous: returns after the
	// goroutine has finished. ctx is the operation deadline; if it expires
	// before exit, Stop returns ctx.Err() and the service eventually reaches
	// READY when the run goroutine finally exits.
	Stop(ctx context.Context) error

	// Terminate transitions any state → TERMINATING (immediate, irreversible).
	// It ends the ACTIVE part for good and releases external resources —
	// listeners, sockets, files. The passive in-memory state is RETAINED, not
	// released: callers still hold pointers (see the type doc), and methods must
	// stay safe to call after Terminate — a service that nil'd its own maps
	// would panic the callers already inside it. Triggers async cleanup; observe
	// Terminated() to know when cleanup truly completes. Once Terminated fires,
	// the service is gone and state field is observationally irrelevant.
	//
	// Implementation guideline:
	//
	//   func (s *Service) Terminate(ctx context.Context) (err error) {
	//       if s.state == svc.StateTERMINATING { return nil }  // idempotent — returns BEFORE the defer arms
	//       prevState := s.state
	//       s.state = svc.StateTERMINATING                      // set IMMEDIATELY (honest to concurrent State() readers)
	//       log.Printf("[INFO][%s] Terminating.", s.Name())     // activity marker
	//       defer func() {
	//           s.terminated <- err                             // ←── THE ONLY send site; fires on EVERY exit path
	//           if err == nil {
	//               log.Printf("[INFO][%s] Terminated.", s.Name())
	//           } else {
	//               log.Printf("[ERROR][%s] Terminated with stop error: %v", s.Name(), err)
	//           }
	//       }()
	//       switch prevState {
	//       case svc.StateRUNNING:
	//           err = s.stop(ctx)        // full stop activity (cancel + wait + logs)
	//       case svc.StateSTOPPING:
	//           err = s.waitStopped(ctx) // wait only — Stop already canceled
	//       }
	//       return err
	//   }
	//
	// Rules:
	//   - Set state = TERMINATING FIRST (before any work), so concurrent State()
	//     reads never see a misleading transient value during shutdown.
	//   - Send on `s.terminated` from the DEFER above, exactly once per service
	//     lifetime (the idempotent branch returns before the defer arms). Never
	//     from Stop, run goroutine exit, RootCancel cascade, or anywhere else.
	//   - The send is UNCONDITIONAL — a missed stop deadline must still report,
	//     with its error. A service that returns early without sending leaves
	//     `framework.Core.WaitServicesTerminated` counting N-1 forever: main
	//     wedges until the supervisor's SIGKILL (reproduced 2026-07-27; one
	//     ctx-blind job wedged the whole process for systemd's full 90s).
	//   - Buffer `s.terminated` with capacity 1 (in NewService) so this single
	//     send never blocks even if no one is reading yet.
	Terminate(ctx context.Context) error

	// Terminated fires once when cleanup truly completes (post-Terminate).
	// Consumed by framework.Core only.
	//
	// Implementation contract:
	//   - Channel type: `chan error`, buffered with capacity 1
	//   - Created in NewService, never re-created
	//   - Sent to ONLY by Terminate (see its doc for the exact send site)
	//   - NEVER close it — the framework treats a receive as the completion
	//     signal; closing would forge spurious zero-valued completions
	Terminated() <-chan error
}
