package framework

import (
	"time"

	"github.com/x64c/gwf/gw/throttle"
)

func (c *Core) PrepareThrottleService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) {
	c.ThrottleService = throttle.NewService(cleanupCycle, cleanupOlderThan)
	c.AddService(c.ThrottleService)
}
