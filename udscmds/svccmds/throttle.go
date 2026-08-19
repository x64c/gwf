package svccmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

// Throttle is the svc-cmd for the throttle service (lifecycle only).
type Throttle struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "throttle"
}

func (h *Throttle) Name() string {
	if h.name == "" {
		return "throttle"
	}
	return h.name
}
func (h *Throttle) Rename(name string) { h.name = name }
func (h *Throttle) Usage() string      { return lifecycleUsage }
func (h *Throttle) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	n := appCore.ThrottleHandle().Node()
	if n.Service() == nil {
		// the absent-handle node: this app never prepared the service
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	return handleLifecycle(appCore, n, subcmd, args, w)
}
