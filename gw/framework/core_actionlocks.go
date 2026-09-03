package framework

import (
	"fmt"

	"github.com/x64c/gwf/gw/locking"
)

// PrepareActionLocks builds the lock manager for the app's action locks from
// the app's sealed coordination mode (see newLockManager).
//
// Whether a lock binds this process or every process running this app is a
// property of the deployment, not of the framework, and a lock that appears
// to be held while another process holds nothing is worse than no lock at
// all. The app states what it is at NewCore, once, and the manager follows
// from that statement, so the two can never disagree. Under CrossProc,
// MainKVDB must be set before this call.
//
// Routes built with the action-lock wrappers refuse to build when this was
// never called, which makes the omission a boot failure instead of a nil
// dereference on the first request that reaches such a route.
func (c *Core) PrepareActionLocks() error {
	m, err := c.newLockManager("action")
	if err != nil {
		return fmt.Errorf("PrepareActionLocks: %w", err)
	}
	c.actionLockingManager = m
	return nil
}

// ActionLockingManager is the manager the app's action locks are held on,
// built by PrepareActionLocks from the sealed coordination mode; nil until
// then.
func (c *Core) ActionLockingManager() locking.Manager { return c.actionLockingManager }
