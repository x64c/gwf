package jobsched

import "time"

type OneTimeJob struct {
	ID       string
	Scope    Scope
	ExecTime time.Time
	Task     func() error
	// Job-specific callbacks
	OnAdded    func()
	OnFinished func(error)
}
