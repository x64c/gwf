package svc

import "io"

// CmdHandler is the contract a per-service svc-cmd must satisfy so it can be
// registered under the umbrella `svc` UDS command.
//
// Implementations are autonomous: they hold their own app handle (typically a
// framework AppProviderFunc) and at Handle time resolve their live service,
// the runtime root context (for Start), and a per-operation context (for
// Stop/Terminate) themselves — so Handle doesn't take them as parameters.
//
// Implementations must support at least "start" and "stop" subcommands and are
// free to add their own (e.g. session's "enable"/"disable", uds's "--force").
type CmdHandler interface {
	// Name is the operator-facing svc-cmd name (e.g. "throttle", "uds"). It is
	// a static string (it must not touch the app/services), so the umbrella can
	// build its routing map at registration time.
	Name() string

	// Rename overrides the default Name(); call before registering.
	Rename(name string)

	// Usage returns a one-line description of the subcommands/flags this
	// svc-cmd accepts (e.g. "start | stop [--ttl <dur>] | status"). Shown by
	// the umbrella's `svc help <name>`.
	Usage() string

	// Handle dispatches a subcommand with its remaining args; output goes to w.
	Handle(subcmd string, args []string, w io.Writer) error
}
