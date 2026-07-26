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
	c.AddService(c.SessionService)
	return nil
}
