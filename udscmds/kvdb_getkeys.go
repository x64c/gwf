package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type KvdbGetKeys struct {
	AppProvider framework.AppProviderFunc
}

func (h *KvdbGetKeys) GroupName() string {
	return "kvdb"
}

func (h *KvdbGetKeys) Command() string {
	return "kvdb-get-keys"
}

func (h *KvdbGetKeys) Desc() string {
	return "Print all the keys in KV database"
}

func (h *KvdbGetKeys) Usage() string {
	return h.Command()
}

func (h *KvdbGetKeys) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	var cursor any = nil
	for {
		keys, nextCursor, err := appCore.MainKVDB.ScanKeys(appCore.RootCtx, cursor, 1000)
		if err != nil {
			return err
		}
		for _, key := range keys {
			_, _ = fmt.Fprintln(w, key)
		}
		if nextCursor == nil {
			break // done
		}
		cursor = nextCursor
	}
	return nil
}
