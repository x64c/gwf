package svccmds

import (
	"fmt"
	"io"
	"slices"

	"github.com/x64c/gwf/gw/framework"
)

// UDS is the svc-cmd for the UDS service. It reuses handleLifecycle but guards
// stop: stopping UDS closes the very socket the operator is on, so it refuses
// unless --force.
type UDS struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "uds"
}

func (h *UDS) Name() string {
	if h.name == "" {
		return "uds"
	}
	return h.name
}
func (h *UDS) Rename(name string) { h.name = name }
func (h *UDS) Usage() string      { return "start | stop [--force] [--ttl <dur>] | status" }

func (h *UDS) Handle(subcmd string, args []string, w io.Writer) error {
	if subcmd == "stop" && !hasForce(args) {
		return fmt.Errorf("refusing to stop UDS over UDS — it would close your own connection; pass --force to override (recover via another transport)")
	}
	appCore := h.AppProvider().AppCore()
	n := appCore.UDSHandle().Node()
	if n.Service() == nil {
		// the absent-handle node: this app never prepared the service
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	return handleLifecycle(appCore, n, subcmd, args, w)
}

func hasForce(args []string) bool {
	return slices.Contains(args, "--force")
}
