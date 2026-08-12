package svccmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

// Web is the svc-cmd for the web (HTTP) service (lifecycle only).
type Web struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "web"
}

func (h *Web) Name() string {
	if h.name == "" {
		return "web"
	}
	return h.name
}
func (h *Web) Rename(name string) { h.name = name }
func (h *Web) Usage() string      { return lifecycleUsage }
func (h *Web) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	if appCore.WebService == nil {
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	return handleLifecycle(appCore, appCore.WebHandle().Node(), subcmd, args, w)
}
