package framework

import (
	"errors"
	"time"

	"github.com/x64c/gwf/gw/web/session"
)

// PrepareSessionService creates the SessionService and registers it with Core.
// Prerequisite: MainKVDB set.
//
// The returned pointer is for BOOT WIRING — reaching the managers while the
// route tree is built. At runtime consumers reach the service through
// SessionHandle; Core exports no raw service field.
func (c *Core) PrepareSessionService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) (*session.Service, error) {
	if c.MainKVDB == nil {
		return nil, errors.New("main kvdb not ready")
	}
	c.sessionService = session.NewService(c.MainKVDB, cleanupCycle, cleanupOlderThan)
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
