package framework

import (
	"time"

	"github.com/x64c/gwf/gw/throttle"
)

// PrepareThrottleService creates the ThrottleService and registers it.
//
// The returned pointer is for BOOT WIRING — SetBucketGroup before Start — and
// for handing into route builders at wrap time. At runtime consumers reach the
// service through ThrottleHandle; Core exports no raw service field.
func (c *Core) PrepareThrottleService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) (*throttle.Service, error) {
	c.throttleService = throttle.NewService(cleanupCycle, cleanupOlderThan)
	// Depends on nothing: a token-bucket engine over string keys, with no
	// service of its own to call. What depends on IT is declared by the
	// dependents.
	node, err := c.RegisterService(c.throttleService)
	if err != nil {
		return nil, err
	}
	c.throttleNode = node
	return c.throttleService, nil
}
