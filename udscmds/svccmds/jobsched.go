package svccmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
)

// JobSched is the svc-cmd for the job scheduler service (lifecycle only).
type JobSched struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "jobsched"
}

func (h *JobSched) Name() string {
	if h.name == "" {
		return "jobsched"
	}
	return h.name
}
func (h *JobSched) Rename(name string) { h.name = name }
func (h *JobSched) Usage() string      { return lifecycleUsage }
func (h *JobSched) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	s := appCore.JobSchedulerService
	if s == nil {
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	return handleLifecycle(appCore.RootCtx, s, subcmd, args, w)
}
