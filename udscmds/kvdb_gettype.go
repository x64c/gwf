package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type KvdbGetType struct {
	AppProvider framework.AppProviderFunc
}

func (h *KvdbGetType) GroupName() string {
	return "kvdb"
}

func (h *KvdbGetType) Command() string {
	return "kvdb-get-type"
}

func (h *KvdbGetType) Desc() string {
	return "Print the type of the given key in KV database"
}

func (h *KvdbGetType) Usage() string {
	return h.Command() + " key"
}

func (h *KvdbGetType) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen != 1 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	key := args[0]
	appCore := h.AppProvider().AppCore()
	found, err := appCore.MainKVDB.Exists(appCore.RootCtx, key)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("key not found")
	}
	typeName, err := appCore.MainKVDB.Type(appCore.RootCtx, key)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintln(w, typeName)
	return nil
}
