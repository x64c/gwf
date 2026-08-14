package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/svc"
)

// Svc is the umbrella UDS command for controlling framework services:
//
//	svc list                       — list controllable services
//	svc help [service]             — usage for one service, or all
//	svc <subcmd> <service> [args]   — route a subcommand to that service
//
// e.g. `svc status web`, `svc stop throttle`, `svc disable session cookie`.
//
// It is built from a list of svc-cmds (the building blocks live in the
// svccmds subpackage); each carries its own Name(). Svc keeps the list in
// registration order (for `list`) plus a name → cmd map for O(1) routing.
type Svc struct {
	cmds   []svc.CmdHandler          // registration order, preserved for listing
	cmdMap map[string]svc.CmdHandler // O(1) routing
}

// NewSvc builds the umbrella from the given svc-cmds, keyed by each one's
// Name(). Duplicate names are reported. Registration order is preserved.
func NewSvc(cmds ...svc.CmdHandler) (*Svc, error) {
	m := make(map[string]svc.CmdHandler, len(cmds))
	for _, c := range cmds {
		if _, dup := m[c.Name()]; dup {
			return nil, fmt.Errorf("svc: duplicate svc-cmd name %q", c.Name())
		}
		m[c.Name()] = c
	}
	return &Svc{cmds: cmds, cmdMap: m}, nil
}

// NewSvcOrPanic is NewSvc for a package-level var, where an error has nowhere
// to go.
// WARNING: This function panics on a duplicate svc-cmd name. Package
// initialization runs before main, so the failure lands as a failed program
// load — nothing has bound a listener or opened a pool yet. See
// sqldbs.NewTableOrPanic for the same rationale.
func NewSvcOrPanic(cmds ...svc.CmdHandler) *Svc {
	h, err := NewSvc(cmds...)
	if err != nil {
		panic(err)
	}
	return h
}

func (h *Svc) GroupName() string { return "svc" }
func (h *Svc) Command() string   { return "svc" }

func (h *Svc) Desc() string {
	return "Control framework services (list / status / start / stop; session protocol enable/disable)"
}

func (h *Svc) Usage() string {
	return "svc list | svc help [service] | svc <subcmd> <service> [args]"
}

func (h *Svc) HandleCommand(args []string, w io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	if args[0] == "list" {
		for _, c := range h.cmds {
			_, _ = fmt.Fprintln(w, c.Name())
		}
		return nil
	}
	if args[0] == "help" {
		if len(args) < 2 {
			// `svc help` — umbrella usage + every svc-cmd's name and usage
			_, _ = fmt.Fprintln(w, h.Usage())
			for _, c := range h.cmds {
				_, _ = fmt.Fprintf(w, "  %-10s %s\n", c.Name(), c.Usage())
			}
			return nil
		}
		// `svc help <service>` — that svc-cmd's usage
		cmd, ok := h.cmdMap[args[1]]
		if !ok {
			return fmt.Errorf("unknown service %q (try `svc list`)", args[1])
		}
		_, _ = fmt.Fprintf(w, "%s: %s\n", cmd.Name(), cmd.Usage())
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	subcmd, serviceName, rest := args[0], args[1], args[2:]
	cmd, ok := h.cmdMap[serviceName]
	if !ok {
		return fmt.Errorf("unknown service %q (try `svc list`)", serviceName)
	}
	return cmd.Handle(subcmd, rest, w)
}
