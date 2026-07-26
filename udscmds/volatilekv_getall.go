package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type VolatilekvGetAll struct {
	AppProvider framework.AppProviderFunc
}

func (h *VolatilekvGetAll) GroupName() string {
	return "volatilekv"
}

func (h *VolatilekvGetAll) Command() string {
	return "volatilekv-get-all"
}

func (h *VolatilekvGetAll) Desc() string {
	return "Print All Volative Key-Values"
}

func (h *VolatilekvGetAll) Usage() string {
	return h.Command()
}

func (h *VolatilekvGetAll) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	volatileKV := appCore.VolatileKV
	if volatileKV == nil {
		return fmt.Errorf("volatile key-value store not ready")
	}
	volatileKV.Range(func(k, v any) bool {
		_, _ = fmt.Fprintf(w, "%s: %v (%T)\n", k.(string), v, v)
		return true
	})
	return nil
}
