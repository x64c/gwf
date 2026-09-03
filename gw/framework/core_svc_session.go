package framework

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/web/session"
)

// PrepareSessionService creates the SessionService and registers it with Core.
// Prerequisite: MainKVDB set.
//
// The returned pointer is for BOOT WIRING — reaching the managers while the
// route tree is built. At runtime consumers reach the service through
// SessionHandle; Core exports no raw service field.
//
// The service's lock manager guards each session's upstream refresh, so
// whether it binds this process or every process running this app decides
// whether two instances may serve one session. That follows from the app's
// sealed coordination mode (see newLockManager), stated once at NewCore.
func (c *Core) PrepareSessionService() (*session.Service, error) {
	if c.MainKVDB == nil {
		return nil, errors.New("main kvdb not ready")
	}
	sessionLocks, err := c.newLockManager("session")
	if err != nil {
		return nil, fmt.Errorf("PrepareSessionService: %w", err)
	}
	c.sessionService = session.NewService(c.MainKVDB, sessionLocks)
	// No dependency to declare: the KVDB it is built with is an infrastructure
	// client, not a service, so nothing in the graph has to outlive it or
	// precede it on that account.
	node, err := c.RegisterService(c.sessionService)
	if err != nil {
		return nil, err
	}
	c.sessionNode = node
	return c.sessionService, nil
}
