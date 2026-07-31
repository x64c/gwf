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

	"github.com/x64c/gwf/gw/svc"
)

// lifecycleUsage is the shared Usage() text for the lifecycle-only svc-cmds.
const lifecycleUsage = "start | stop [--ttl <dur>] | status"

// handleLifecycle dispatches start/stop/status for a plain svc.Service. rootCtx
// is the runtime parent: Start derives the service's lifetime from it, and the
// Stop op-ctx is derived from it too (so app shutdown cascades into a Stop) —
// see stopCtx. Shared by the lifecycle-only svc-cmds (throttle, web, jobsched)
// and uds.
func handleLifecycle(rootCtx context.Context, s svc.Service, subcmd string, args []string, w io.Writer) error {
	switch subcmd {
	case "start":
		if err := s.Start(rootCtx); err != nil {
			return err
		}
	case "stop":
		opCtx, cancel := stopCtx(rootCtx, args)
		defer cancel()
		if err := s.Stop(opCtx); err != nil {
			return err
		}
	case "status":
		// fall through to status print
	default:
		return fmt.Errorf("unknown subcommand %q (supported: start, stop, status)", subcmd)
	}
	_, _ = fmt.Fprintf(w, "%s: %s\n", s.Name(), s.State())
	return nil
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
