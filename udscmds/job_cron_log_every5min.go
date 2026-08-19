package udscmds

import (
	"fmt"
	"io"
	"log"
	"strings"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/jobsched"
)

type JobCronLogEvery5Min struct {
	AppProvider framework.AppProviderFunc
}

func (*JobCronLogEvery5Min) GroupName() string {
	return "job"
}

func (h *JobCronLogEvery5Min) Command() string {
	return "job-cron-log-every5min"
}

func (h *JobCronLogEvery5Min) Desc() string {
	return "[TEST] Add a cron job to log a message every 5 minute"
}

func (h *JobCronLogEvery5Min) Usage() string {
	return h.Command() + " message"
}

func (h *JobCronLogEvery5Min) HandleCommand(args []string, w io.Writer) error {
	argLen := len(args)
	if argLen < 1 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	msg := strings.Join(args, " ")
	jobID := "log-msg-every-5min"
	cronjob := jobsched.NewEveryMinEmptyCronJob(jobID)
	cronjob.Minutes = jobsched.BitsFromMinutes([]int{5, 10, 15, 20, 25, 30, 35, 40, 45, 50, 55})
	cronjob.Task = func() error {
		log.Printf("[CRON] message: %s", msg)
		return nil
	}
	appCore := h.AppProvider().AppCore()
	// Registering a job is a CONSUMER use, so it goes through the gate: an
	// un-admitted scheduler refuses instead of quietly queuing work.
	jobSched, ok := appCore.JobSchedulerHandle().Get()
	if !ok {
		return fmt.Errorf("job scheduler service unavailable")
	}
	err := jobSched.AddCronJob(cronjob)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "cron job %q scheduled", jobID)
	return nil
}
