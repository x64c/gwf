package framework

import "github.com/x64c/gwf/gw/jobsched"

// PrepareJobSchedulerService creates the JobSchedulerService and registers it.
//
// It returns an error where it previously returned nothing: registration can
// now fail — a duplicate name, a dependency that is not registered — and a
// registration that failed silently would leave a service outside the graph,
// which is a service with no ordering guarantee and no admission. The signature
// is catching up with what the call can do.
func (c *Core) PrepareJobSchedulerService() error {
	c.JobSchedulerService = jobsched.NewService()
	// Depends on nothing at this level. Individual jobs may reach for whatever
	// the app hands them, but a job's needs are the job's to declare, not the
	// scheduler's — the scheduler only runs what it is given.
	_, err := c.RegisterService(c.JobSchedulerService)
	return err
}
