package framework

import (
	"errors"
	"time"

	"github.com/x64c/gwf/gw/web/session"
)

// PrepareSessionService creates the SessionService and registers it with Core.
// Prerequisite: MainKVDB set.
func (c *Core) PrepareSessionService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) error {
	if c.MainKVDB == nil {
		return errors.New("main kvdb not ready")
	}
	c.SessionService = session.NewService(c.MainKVDB, cleanupCycle, cleanupOlderThan)
	// No dependency to declare: the KVDB it is built with is an infrastructure
	// client, not a service, so nothing in the graph has to outlive it or
	// precede it on that account.
	if _, err := c.RegisterService(c.SessionService); err != nil {
		return err
	}
	return nil
}
