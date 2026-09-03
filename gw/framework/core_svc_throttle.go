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
//
// Two static types, one object: boot wiring gets the concrete service, whose
// surface it needs, while Core holds it as a throttle.Limiter and hands that
// on. What answers the rate question is therefore settled HERE, at Prepare —
// no request path asks which implementation it got.
func (c *Core) PrepareThrottleService(cleanupCycle time.Duration, cleanupOlderThan time.Duration) (*throttle.Service, error) {
	service := throttle.NewService(cleanupCycle, cleanupOlderThan)
	// Depends on nothing: a token-bucket engine over string keys, with no
	// service of its own to call. What depends on IT is declared by the
	// dependents.
	node, err := c.RegisterService(service)
	if err != nil {
		return nil, err
	}
	c.throttleService = service // the Limiter seat, taken only once registration held
	c.throttleNode = node
	return service, nil
}
