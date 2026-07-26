package framework

import "github.com/x64c/gwf/gw/jobsched"

func (c *Core) PrepareJobSchedulerService() {
	c.JobSchedulerService = jobsched.NewService()
	c.AddService(c.JobSchedulerService)
}
