package framework

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/svc"
	"github.com/x64c/gwf/gw/throttle"
)

// newLimiter builds the app's rate limiter from its sealed coordination mode,
// with the conf's bucket groups already set on it, so that a limit binds exactly
// what the mode says the app is: one process, or every process running it.
//
//	InProc    → LimiterInMemBuckets, sweeping on the conf's cycle
//	CrossProc → LimiterKVDBBuckets over MainKVDB, its buckets' region being
//	            "<appName>:th:"; MainKVDB must be set first
//
// A limit is a claim about the app, not about one of its processes, and one that
// admits its number per process while saying per app is worse than no limit at
// all. The app states what it is at NewCore, once, and the limiter follows from
// that statement, so the two can never disagree.
func (c *Core) newLimiter(conf throttle.Conf) (throttle.Limiter, error) {
	bucketConfs := conf.BucketConfs()
	switch c.coordMode {
	case coord.InProc:
		limiter := throttle.NewLimiterInMemBuckets(conf.CleanupCycle(), conf.CleanupOlderThan())
		for id, bucketConf := range bucketConfs {
			if err := limiter.SetBucketGroup(id, bucketConf); err != nil {
				return nil, err
			}
		}
		return limiter, nil
	case coord.CrossProc:
		if c.MainKVDB == nil {
			return nil, errors.New("main kvdb not ready — under CrossProc the buckets live in it")
		}
		limiter := throttle.NewLimiterKVDBBuckets(c.MainKVDB, c.appName+":th:")
		for id, bucketConf := range bucketConfs {
			if err := limiter.SetBucketGroup(id, bucketConf); err != nil {
				return nil, err
			}
		}
		return limiter, nil
	default:
		return nil, fmt.Errorf("unknown coordination mode %v", c.coordMode)
	}
}

// PrepareThrottleService builds the limiter the conf describes and registers it.
//
// The conf carries what a limiter is made of — its bucket groups, and the sweep
// the in-memory one needs — so the rates an app enforces are a setting rather
// than a line of its code. Load it with throttle.LoadConf; a caller with no file,
// a rig say, hands over a conf it built itself.
//
// Bucket groups are BOOT WIRING: set here, before the service starts, and only
// read afterwards. At runtime consumers reach the limiter through ThrottleHandle;
// Core exports no raw service field.
//
//   - Returns: nil; an error when the conf is invalid, the mode's limiter cannot
//     be built, a group is refused, or registration is.
//   - Uses: throttle.Conf.Validate, framework.Core.newLimiter,
//     framework.Core.RegisterService.
func (c *Core) PrepareThrottleService(conf throttle.Conf) error {
	if err := conf.Validate(); err != nil {
		return fmt.Errorf("PrepareThrottleService: %w", err)
	}
	limiter, err := c.newLimiter(conf)
	if err != nil {
		return fmt.Errorf("PrepareThrottleService: %w", err)
	}
	service, ok := limiter.(svc.Service)
	if !ok {
		return fmt.Errorf("PrepareThrottleService: %T is no service, so it cannot be registered", limiter)
	}
	// Depends on nothing: a token-bucket engine over string keys, with no
	// service of its own to call. What depends on IT is declared by the
	// dependents.
	node, err := c.RegisterService(service)
	if err != nil {
		return fmt.Errorf("PrepareThrottleService: %w", err)
	}
	c.throttleService = limiter // the Limiter seat, taken only once registration held
	c.throttleNode = node
	return nil
}
