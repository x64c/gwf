package framework

import (
	"time"

	"github.com/x64c/gwf/gw/throttle"
)

// PrepareThrottleService creates the ThrottleService and registers it.
//
// It returns an error where it previously returned nothing: registration can
// now fail — a duplicate name, a dependency that is not registered — and a
// registration that failed silently would leave a service outside the graph,
// which is a service with no ordering guarantee and no admission. The signature
// is catching up with what the call can do.
func (c *Core) PrepareThrottleService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) error {
	c.ThrottleService = throttle.NewService(cleanupCycle, cleanupOlderThan)
	// Depends on nothing: a token-bucket engine over string keys, with no
	// service of its own to call. What depends on IT is declared by the
	// dependents.
	node, err := c.RegisterService(c.ThrottleService)
	if err != nil {
		return err
	}
	c.throttleNode = node
	return nil
}
