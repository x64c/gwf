package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

type SqldbPrintRawStore struct {
	AppProvider framework.AppProviderFunc
}

func (h *SqldbPrintRawStore) GroupName() string {
	return "sqldb"
}

func (h *SqldbPrintRawStore) Command() string {
	return "sql-print-rawstore"
}

func (h *SqldbPrintRawStore) Desc() string {
	return "Print stored raw SQL statements"
}

func (h *SqldbPrintRawStore) Usage() string {
	return h.Command() + " clientname storename [storekey]"
}

func (h *SqldbPrintRawStore) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen < 2 || argLen > 3 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	appCore := h.AppProvider().AppCore()
	dbClient, ok := appCore.SQLDBClients[args[0]]
	if !ok {
		return fmt.Errorf("db client not found: %s", args[0])
	}
	rawSQLStore := dbClient.RawSQLStore(args[1])
	if rawSQLStore == nil {
		return fmt.Errorf("raw sql store not found: %s", args[1])
	}

	if argLen == 2 {
		stmts := rawSQLStore.GetAll()
		for k, v := range stmts {
			_, _ = fmt.Fprintf(w, "\n%q:\n%s\n\n", k, v)
		}
		return nil
	}

	storeKey := args[2]
	stmt, exists := rawSQLStore.Get(storeKey)
	if !exists {
		return fmt.Errorf("\n%q not found\n", storeKey)
	}
	_, _ = fmt.Fprintf(w, "\n%q:\n%s\n\n", storeKey, stmt)
	return nil
}
