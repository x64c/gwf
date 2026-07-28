package svc

import "context"

type Service interface {
	Name() string
	State() State

	// Start transitions READY → RUNNING. parentCtx is the runtime cancellation
	// lineage (typically Core.RootCtx); the service derives its own per-cycle
	// context from it. Returns a bootstrap error if start fails.
	Start(parentCtx context.Context) error

	// Stop transitions RUNNING → STOPPING (immediate) → READY (when the run
	// goroutine has exited). Synchronous: returns after the goroutine has
	// finished. ctx is the operation deadline; if it expires before exit, Stop
	// returns ctx.Err() and the service eventually reaches READY when the run
	// goroutine finally exits.
	Stop(ctx context.Context) error

	// Terminate transitions any state → TERMINATING (immediate, irreversible).
	// Releases all resources, including prepared state. Triggers async cleanup;
	// observe Terminated() to know when cleanup truly completes. Once Terminated
	// fires, the service is gone and state field is observationally irrelevant.
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
	//           err = s.waitStopped(ctx) // wait only — Stop already cancelled
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
