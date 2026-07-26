package udscmds

import (
	"fmt"
	"io"
	"strconv"

	"github.com/x64c/gwf/gw/framework"
)

type VolatilekvSet struct {
	AppProvider framework.AppProviderFunc
}

func (h *VolatilekvSet) GroupName() string {
	return "volatilekv"
}

func (h *VolatilekvSet) Command() string {
	return "volatilekv-set"
}

func (h *VolatilekvSet) Desc() string {
	return "Set Volative Key-Value"
}

func (h *VolatilekvSet) Usage() string {
	return h.Command() + " key value type"
}

func (h *VolatilekvSet) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen != 3 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	appCore := h.AppProvider().AppCore()
	volatileKV := appCore.VolatileKV
	if volatileKV == nil {
		return fmt.Errorf("volatile key-value store not ready")
	}
	key := args[0]
	valStr := args[1]
	typeStr := args[2]
	switch typeStr {
	case "string":
		volatileKV.Store(key, valStr)
	case "int64":
		val, err := strconv.ParseInt(valStr, 10, 64)
		if err != nil {
			return fmt.Errorf("failed to parse %s to int64. %v", valStr, err)
		}
		volatileKV.Store(key, val)
	default:
		return fmt.Errorf("supported types: string, int64")
	}
	_, _ = fmt.Fprintln(w, "volatile key-value stored")
	return nil
}
