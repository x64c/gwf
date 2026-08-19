package framework

import "github.com/x64c/gwf/gw/jobsched"

// PrepareJobSchedulerService creates the JobSchedulerService and registers it.
//
// The returned pointer is for BOOT WIRING — SetCallbacks/UseDefaultLoggers
// before Start. At runtime consumers reach the service through
// JobSchedulerHandle; Core exports no raw service field.
func (c *Core) PrepareJobSchedulerService() (*jobsched.Service, error) {
	c.jobSchedulerService = jobsched.NewService()
	// Depends on nothing at this level. Individual jobs may reach for whatever
	// the app hands them, but a job's needs are the job's to declare, not the
	// scheduler's — the scheduler only runs what it is given.
	node, err := c.RegisterService(c.jobSchedulerService)
	if err != nil {
		return nil, err
	}
	c.jobSchedulerNode = node
	return c.jobSchedulerService, nil
}
