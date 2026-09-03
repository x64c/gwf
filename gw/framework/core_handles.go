package framework

import (
	"github.com/x64c/gwf/gw/jobsched"
	"github.com/x64c/gwf/gw/throttle"
	"github.com/x64c/gwf/gw/uds"
	"github.com/x64c/gwf/gw/web"
	"github.com/x64c/gwf/gw/web/session"
)

// The *Handle accessors mint gated references to Core's own services — the way
// a consumer reaches one (svc.Service: whether a caller may use a service is
// decided in front of the pointer, by the framework). Each handle is bound to
// the node retained when the service was prepared; an app that never prepared
// the service gets a handle that reports unavailable forever, so a consumer's
// no-service path is the same code as its service-unavailable path.
//
// Resolve the handle once at wiring time (a wrapper's Wrap, a job's
// construction) and ask Get per use: resolution is boot-shaped, the per-use
// question is one atomic load.

// UDSHandle returns the gated reference to the UDS admin-socket service.
func (c *Core) UDSHandle() ServiceHandle[*uds.Service] {
	if c.udsNode == nil {
		return absentServiceHandle[*uds.Service]()
	}
	return newServiceHandle(c.udsNode, c.udsService)
}

// JobSchedulerHandle returns the gated reference to the job-scheduler service.
func (c *Core) JobSchedulerHandle() ServiceHandle[*jobsched.Service] {
	if c.jobSchedulerNode == nil {
		return absentServiceHandle[*jobsched.Service]()
	}
	return newServiceHandle(c.jobSchedulerNode, c.jobSchedulerService)
}

// WebHandle returns the gated reference to the web service.
func (c *Core) WebHandle() ServiceHandle[*web.Service] {
	if c.webNode == nil {
		return absentServiceHandle[*web.Service]()
	}
	return newServiceHandle(c.webNode, c.webService)
}

// SessionHandle returns the gated reference to the session service.
func (c *Core) SessionHandle() ServiceHandle[*session.Service] {
	if c.sessionNode == nil {
		return absentServiceHandle[*session.Service]()
	}
	return newServiceHandle(c.sessionNode, c.sessionService)
}

// ThrottleHandle returns the gated reference to the throttle limiter.
func (c *Core) ThrottleHandle() ServiceHandle[throttle.Limiter] {
	if c.throttleNode == nil {
		return absentServiceHandle[throttle.Limiter]()
	}
	return newServiceHandle(c.throttleNode, c.throttleService)
}
