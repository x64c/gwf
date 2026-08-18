package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type KvdbGet struct {
	AppProvider framework.AppProviderFunc
}

func (h *KvdbGet) GroupName() string {
	return "kvdb"
}

func (h *KvdbGet) Command() string {
	return "kvdb-get"
}

func (h *KvdbGet) Desc() string {
	return "Print the value of the given key in KV database"
}

func (h *KvdbGet) Usage() string {
	return h.Command() + " key"
}

func (h *KvdbGet) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen != 1 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	key := args[0]
	appCore := h.AppProvider().AppCore()
	ctx := appCore.RootCtx
	typeName, err := appCore.MainKVDB.Type(ctx, key)
	if err != nil {
		return err
	}
	switch typeName {
	case "string":
		strVal, found, err := appCore.MainKVDB.GetValue(ctx, key)
		if err != nil {
			return err
		}
		if !found {
			return fmt.Errorf("key not found")
		}
		_, _ = fmt.Fprintln(w, strVal)
	case "list":
		vals, err := appCore.MainKVDB.ListRange(ctx, key, 0, -1)
		if err != nil {
			return err
		}
		for _, v := range vals {
			_, _ = fmt.Fprintln(w, v)
		}
	case "hash":
		valMap, err := appCore.MainKVDB.HashGetAll(ctx, key)
		if err != nil {
			return err
		}
		for k, v := range valMap {
			_, _ = fmt.Fprintf(w, "%s: %s\n", k, v)
		}
	default:
		return fmt.Errorf("unsupported type: %s", typeName)
	}
	return nil
}
