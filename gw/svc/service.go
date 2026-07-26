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
	//   func (s *Service) Terminate(ctx context.Context) error {
	//       if s.state == svc.StateTERMINATING { return nil }  // idempotent
	//       prevState := s.state
	//       s.state = svc.StateTERMINATING                      // set IMMEDIATELY (honest to concurrent State() readers)
	//       log.Printf("[INFO][%s] Terminating.", s.Name())     // activity marker
	//       switch prevState {
	//       case svc.StateRUNNING:
	//           if err := s.stop(ctx); err != nil { return err }     // full stop activity (cancel + wait + logs)
	//       case svc.StateSTOPPING:
	//           if err := s.waitStopped(ctx); err != nil { return err } // wait only — Stop already cancelled
	//       }
	//       s.terminated <- nil                                  // ←── THE ONLY allowed send site for s.terminated
	//       log.Printf("[INFO][%s] Terminated.", s.Name())       // completion marker
	//       return nil
	//   }
	//
	// Rules:
	//   - Set state = TERMINATING FIRST (before any work), so concurrent State()
	//     reads never see a misleading transient value during shutdown.
	//   - Send on `s.terminated` ONLY HERE, exactly once per service lifetime.
	//     Never from Stop, run goroutine exit, RootCancel cascade, or anywhere
	//     else. The one-shot semantic is what `framework.Core.WaitServicesTerminated`
	//     depends on to count N completions.
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
