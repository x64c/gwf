// Package svccmds provides the per-service svc-cmds (svc.CmdHandler building
// blocks) wired into the umbrella udscmds.Svc command. Each is an autonomous
// instance (like a middleware): it holds an AppProviderFunc and resolves its
// live service + contexts itself at command time.
package svccmds

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/x64c/gwf/gw/framework"
)

// lifecycleUsage is the shared Usage() text for the lifecycle-only svc-cmds.
const lifecycleUsage = "start | stop [--ttl <dur>] | status"

// handleLifecycle dispatches start/stop/status for one registered service,
// through Core's operator ops so ADMISSION follows the action: stop revokes it
// before the service pauses, start grants it back on success. Calling
// Start/Stop on the raw service instead would move the phase and leave the
// app-level verdict behind — every gated consumer would keep its old answer.
//
// The Stop op-ctx derives from Core's RootCtx (so app shutdown cascades into a
// Stop) — see stopCtx. Shared by the lifecycle-only svc-cmds (throttle, web,
// jobsched, uds) and by session's lifecycle half.
func handleLifecycle(appCore *framework.Core, n *framework.ServiceNode, subcmd string, args []string, w io.Writer) error {
	if err := doLifecycle(appCore, n, subcmd, args); err != nil {
		return err
	}
	printLifecycleStatus(n, w)
	return nil
}

// doLifecycle runs the lifecycle action without printing — for svc-cmds with
// their own status shape (session). "status" is a no-op: the caller prints.
func doLifecycle(appCore *framework.Core, n *framework.ServiceNode, subcmd string, args []string) error {
	switch subcmd {
	case "start":
		return appCore.StartServiceNode(n)
	case "stop":
		opCtx, cancel := stopCtx(appCore.RootCtx, args)
		defer cancel()
		return appCore.StopServiceNode(opCtx, n)
	case "status":
		return nil
	default:
		return fmt.Errorf("unknown subcommand %q (supported: start, stop, status)", subcmd)
	}
}

// printLifecycleStatus prints one service's phase beside the app-level verdict.
// The two legitimately differ (svc.Service vs ServiceNode): a stopped service
// still reports its own phase while admission — what gated consumers actually
// ask — is already false.
func printLifecycleStatus(n *framework.ServiceNode, w io.Writer) {
	s := n.Service()
	_, _ = fmt.Fprintf(w, "%s: %s (admitted=%v)\n", s.Name(), s.State(), n.Admitted())
}

// stopCtx derives the Stop operation context from rootCtx.
//
// By default there is NO deadline: Stop runs to completion, bounded only by app
// shutdown (rootCtx canceling cascades in). A default timeout is deliberately
// avoided — it could cut off a legitimately long Stop (e.g. tearing down real
// resources). The operator opts into a hard bound with "--ttl <duration>".
func stopCtx(rootCtx context.Context, args []string) (context.Context, context.CancelFunc) {
	if ttl, ok := parseTTL(args); ok {
		return context.WithTimeout(rootCtx, ttl)
	}
	return rootCtx, func() {} // no deadline; rootCtx (app shutdown) is the only bound
}

// parseTTL looks for "--ttl <duration>" in args (e.g. "--ttl 60s") and returns
// the parsed duration. Returns ok=false if absent or unparseable.
func parseTTL(args []string) (time.Duration, bool) {
	for i, a := range args {
		if a == "--ttl" && i+1 < len(args) {
			if d, err := time.ParseDuration(args[i+1]); err == nil {
				return d, true
			}
		}
	}
	return 0, false
}
