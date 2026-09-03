package framework

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/locking"
)

// newLockManager builds the lock manager named name from the app's sealed
// coordination mode, so that a lock binds exactly what the mode says the app
// is: one process, or every process running it.
//
//	InProc    → ManagerLocalLedgerOnly
//	CrossProc → ManagerKVDBLedger over MainKVDB, its external ledger's region
//	            being "<appName>:lk:<name>:"; MainKVDB must be set first
//
// The manager's name is what it serves — "action" for the action locks,
// "session" for the session locks — and under CrossProc it carves the
// region, so two managers of one app never share a row.
func (c *Core) newLockManager(name string) (locking.Manager, error) {
	switch c.coordMode {
	case coord.InProc:
		return locking.NewManagerLocalLedgerOnly(name)
	case coord.CrossProc:
		if c.MainKVDB == nil {
			return nil, errors.New("main kvdb not ready — under CrossProc the lock managers live in it")
		}
		return locking.NewManagerKVDBLedger(name, c.MainKVDB, c.appName+":lk:"+name+":")
	default:
		return nil, fmt.Errorf("unknown coordination mode %v", c.coordMode)
	}
}
