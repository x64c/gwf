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
	n := appCore.JobSchedulerHandle().Node()
	if n.Service() == nil {
		// the absent-handle node: this app never prepared the service
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	return handleLifecycle(appCore, n, subcmd, args, w)
}
