package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/kvdbs"
)

type KvdbGetTTL struct {
	AppProvider framework.AppProviderFunc
}

func (h *KvdbGetTTL) GroupName() string {
	return "kvdb"
}

func (h *KvdbGetTTL) Command() string {
	return "kvdb-get-ttl"
}

func (h *KvdbGetTTL) Desc() string {
	return "Print TTL of the given key in KV database"
}

func (h *KvdbGetTTL) Usage() string {
	return h.Command() + " key"
}

func (h *KvdbGetTTL) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen != 1 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	key := args[0]
	appCore := h.AppProvider().AppCore()
	ttl, state, err := appCore.MainKVDB.TTL(appCore.RootCtx, key)
	if err != nil {
		return err
	}
	switch state {
	case kvdbs.TTLKeyNotFound:
		return fmt.Errorf("key not found")
	case kvdbs.TTLPersistent:
		_, _ = fmt.Fprintln(w, "persistent")
	case kvdbs.TTLExpiring:
		_, _ = fmt.Fprintf(w, "%v (%ds)\n", ttl, int64(ttl.Seconds()))
	default:
		return fmt.Errorf("invalid TTLState")
	}
	return nil
}
