package framework

import (
	"errors"
	"fmt"

	"github.com/x64c/gwf/gw/coord"
	"github.com/x64c/gwf/gw/jobsched"
)

// newKVDBJobLedger builds the KVDB job ledger the app's sealed coordination
// mode calls for, so that a job's scope binds exactly what the mode says the
// app is:
//
//	CrossProc → KVDBLedger over MainKVDB, its marks under "<appName>:jb:"
//	InProc    → an error: under InProc a OncePerApp job collapses into this
//	            instance and never reaches a ledger, so being asked for one is a bug
//
// It is passed to the service as a provider, not called here: whether an app
// needs a ledger depends on the jobs it registers, so this runs at the first
// OncePerApp registration — by which time MainKVDB exists, whatever order the
// app prepared things in.
func (c *Core) newKVDBJobLedger() (jobsched.LedgerForJobsPerAppCrossProc, error) {
	switch c.coordMode {
	case coord.InProc:
		return nil, errors.New("no job ledger under InProc — a OncePerApp job runs in this instance")
	case coord.CrossProc:
		if c.MainKVDB == nil {
			return nil, errors.New("main kvdb not ready — under CrossProc the job ledger lives in it")
		}
		return jobsched.NewKVDBLedger(c.MainKVDB, c.appName+":jb:")
	default:
		return nil, fmt.Errorf("unknown coordination mode %v", c.coordMode)
	}
}

// PrepareJobSchedulerService creates the JobSchedulerService and registers it.
//
// The scheduler is built with the app's sealed coordination mode, because a
// job's Scope is read against it — what "once per app" means depends on what
// the app says it is.
//
// The returned pointer is for BOOT WIRING — SetCallbacks/UseDefaultLoggers
// before Start. At runtime consumers reach the service through
// JobSchedulerHandle; Core exports no raw service field.
func (c *Core) PrepareJobSchedulerService() (*jobsched.Service, error) {
	jobSched, err := jobsched.NewService(c.coordMode, c.newKVDBJobLedger)
	if err != nil {
		return nil, fmt.Errorf("PrepareJobSchedulerService: %w", err)
	}
	c.jobSchedulerService = jobSched
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
