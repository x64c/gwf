package svccmds

import (
	"fmt"
	"io"
	"strings"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/web/session"
)

// Session is the svc-cmd for the session service. Beyond the lifecycle verbs it
// exposes per-protocol enable/disable, since the session service hosts
// independently switchable protocols (cookie, bearer).
type Session struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "session"
}

func (h *Session) Name() string {
	if h.name == "" {
		return "session"
	}
	return h.name
}
func (h *Session) Rename(name string) { h.name = name }
func (h *Session) Usage() string {
	return "start | stop [--ttl <dur>] | status | enable <protocol> | disable <protocol>"
}

func (h *Session) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	s := appCore.SessionService
	if s == nil {
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	node := appCore.SessionHandle().Node()
	switch subcmd {
	case "start", "stop":
		// Lifecycle goes through Core's operator ops so admission follows the
		// action; the session-specific status shape prints below.
		if err := doLifecycle(appCore, node, subcmd, args); err != nil {
			return err
		}
	case "enable":
		return setSessionProtocol(node, s, args, true, w)
	case "disable":
		return setSessionProtocol(node, s, args, false, w)
	case "status":
		// fall through to status print
	default:
		return fmt.Errorf("unknown subcommand %q (supported: start, stop, status, enable <protocol>, disable <protocol>)", subcmd)
	}
	printSessionStatus(node, s, w)
	return nil
}

func setSessionProtocol(n *framework.ServiceNode, s *session.Service, args []string, enable bool, w io.Writer) error {
	names := sessionProtocolNames(s)
	if len(args) < 1 {
		return fmt.Errorf("missing protocol name (configured: %s)", strings.Join(names, ", "))
	}
	p := sessionProtocol(s, args[0])
	if p == nil {
		return fmt.Errorf("unknown or unconfigured protocol %q (configured: %s)", args[0], strings.Join(names, ", "))
	}
	if enable {
		p.Enable()
	} else {
		p.Disable()
	}
	printSessionStatus(n, s, w)
	return nil
}

// sessionProtocol returns the named protocol's manager (a svc.Switchable), or
// nil if not configured.
func sessionProtocol(s *session.Service, name string) svc.Switchable {
	switch name {
	case "cookie":
		if s.CookieSessionManager != nil {
			return s.CookieSessionManager
		}
	case "bearer":
		if s.BearerSessionManager != nil {
			return s.BearerSessionManager
		}
	}
	return nil
}

func sessionProtocolNames(s *session.Service) []string {
	var names []string
	if s.CookieSessionManager != nil {
		names = append(names, "cookie")
	}
	if s.BearerSessionManager != nil {
		names = append(names, "bearer")
	}
	return names
}

func printSessionStatus(n *framework.ServiceNode, s *session.Service, w io.Writer) {
	printLifecycleStatus(n, w)
	for _, name := range sessionProtocolNames(s) {
		p := sessionProtocol(s, name)
		enabled := "disabled"
		if p.Enabled() {
			enabled = "enabled"
		}
		// "serving" is the wrapper gate's actual verdict: admitted && enabled.
		serving := "not-serving"
		if n.Admitted() && p.Enabled() {
			serving = "serving"
		}
		_, _ = fmt.Fprintf(w, "  %s: %s (%s)\n", name, enabled, serving)
	}
}
