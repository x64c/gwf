package jobsched

import "context"

// LedgerForJobsPerAppCrossProc records that a job's planned minute has been
// taken, so that among the instances of one app exactly one of them runs it.
// A planned minute is the
// pair (job id, minute) — a cron job has one per matching minute, a one-time
// job exactly one — and it is what every instance can name identically without
// asking anyone.
//
// Claim's error is not a verdict. It reports that NO verdict could be computed
// — what a ledger outside this process has to report while it is unreachable —
// and a planned minute that could not be claimed must not be run: instances
// proceeding on "I could not ask" is the duplicate this ledger exists to
// prevent.
type LedgerForJobsPerAppCrossProc interface {
	Claim(ctx context.Context, jobID string, plannedMinute int64) (won bool, err error)
}

// LedgerForJobsPerAppCrossProcProvider yields the ledger the service should
// claim in. It is asked the FIRST time an OncePerApp job is registered and
// never before: whether an
// app needs a ledger depends on the jobs it registers, not on its coordination
// mode, so a CrossProc app whose jobs are all OncePerInstance must not be made
// to have one.
//
// A failure is not remembered. An app that prepares its store after its
// scheduler can still register a per-app job later and be answered correctly.
type LedgerForJobsPerAppCrossProcProvider func() (LedgerForJobsPerAppCrossProc, error)
