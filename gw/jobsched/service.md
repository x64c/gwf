# Job Scheduler Service

Minute-precision scheduler for one-time and cron jobs. Apps register jobs
via `AddOneTimeJob(job)` / `AddCronJob(job)`; a background goroutine
fires due jobs at each minute tick.

This service consists of 2 things:

1. **Job registry** — `oneTimeJobs` + `cronJobs` maps + Add/Delete/Get
   methods + callback configuration (`OnOneTimeJobAdded`,
   `OnCronJobFinished`, ...). The registration / inspection API.
2. **Background scheduler service** — a goroutine that fires every minute,
   dispatches due jobs in worker goroutines, and recovers from panics.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 is available from the moment `Prepare` returns and stays available for
the lifetime of the process.

## Prepare

`Core.PrepareJobSchedulerService()`:
- Constructs the Service (`NewService`).
- Initialises empty `oneTimeJobs` and `cronJobs` maps.
- Registers the Service with `Core.RegisterService`, which places it in the
  composition graph the start and terminate walks follow.

After `Prepare`:
- The app may call `UseDefaultLoggers()` to wire default logging callbacks
  for added/finished events.
- The app registers jobs via `AddOneTimeJob` / `AddCronJob` — these do
  **not** check service state, so jobs can be added at any point (before
  Start, between Stop and Start, or while running).

## Start()

Spawns the background scheduler goroutine — a `time.Ticker` at 1-minute
resolution. On each tick:
- Scans `oneTimeJobs` for entries whose minute-key matches `now`,
  dispatches each in a worker goroutine (`s.wg.Go`), removes from
  the registry.
- Walks `cronJobs` for entries whose spec matches `now`, dispatches each
  similarly.
- Wraps the per-tick body in a `recover()` so a panicking job doesn't
  kill the scheduler.

## Timing: minute precision, at the launch second

The ticker is created when `Start` runs and fires every 60 s from that
instant, so every tick lands at the same second of the minute — the second
`Start` happened at. Nothing aligns it to :00. A service started at
15:19:31 ticks at 15:20:31, 15:21:31, …; each `Stop`/`Start` cycle makes a
new ticker, and a new reference second.

What that means for a job:
- A cron job matches on the tick's calendar minute (`Matches(now)`: minute,
  hour, day of month, weekday), so it fires once per matching minute, at
  the launch second — "every 5 minutes" is 15:20:31, 15:25:31, …
- A one-time job is keyed by its minute: `AddOneTimeJob` refuses an
  `ExecTime` less than 30 s ahead, rounds one carrying seconds UP to the
  next whole minute, and the job fires at that minute's tick — 0 to 59 s
  after the minute began, never before `ExecTime`.
- Each dispatch runs the task on its own goroutine and nothing serializes
  occurrences: a task that outlives its interval runs beside its next one.

The precision is the minute; the second within it belongs to the launch.
A particular second, or a period under a minute, is not something this
service can give.

## Stop()

Halts the scheduler goroutine. The data plane is *partly* affected:

What keeps working:
- `AddOneTimeJob` / `AddCronJob` / `DeleteOneTimeJob` / `DeleteCronJob` —
  registry mutations succeed normally.
- `GetOneTimeJobs` / `GetCronJobs` — inspection returns the current
  registry contents.

What stops:
- The minute ticker is no longer firing, so **no job is dispatched while
  stopped**. Jobs added during a stop sit in the registry and execute
  only after `Start()` resumes the goroutine.

Stop is **graceful**: the `run` goroutine waits on `s.wg.Wait()` after its
ctx is cancelled, so any worker goroutines already running a dispatched
job finish before the `stopped` channel closes.

`Start()` again resumes the ticker; the registry contents (including
jobs added during the pause) become eligible to fire on the next tick.

## Terminate()

Same flow as `Stop()` (cancel + graceful `wg.Wait`), plus:
- Terminal: state stays `TERMINATING`. The service cannot be `Start`ed
  again in this process lifetime.
- Fires the framework's `Terminated` channel so
  `Core.WaitServicesTerminated` can count this service as done.

No scheduler-specific resources to release — the job maps and any
pending entries are reclaimed by GC at process exit. Jobs that hadn't
yet fired never run.

## Operator note

"Stop jobsched" pauses **execution**, not registration. The registry API
keeps accepting Add/Delete/Get calls; nothing fires until Start resumes
the scheduler. Useful for maintenance windows — stop, drain currently
running jobs (`wg.Wait` handles this automatically), do operational work,
then start again to resume firing.

**Missed one-time jobs are not caught up.** Each tick only checks
`oneTimeJobs[key]` for the current minute (`key = now.Unix() / 60`); past
keys are never re-visited. A one-time job whose `ExecTime` fell during a
Stop window will sit in the map under its stale key forever — it never
fires, *and* it never gets pruned (no code path visits past keys).

That's a **memory leak**: the map grows by one entry per missed one-time
job, and the only thing reclaiming them is process exit. Per-entry cost
is small (a `*OneTimeJob` plus a slice header), but a long Stop window
with many registered one-time jobs accumulates indefinitely.

Cron jobs behave differently: each tick walks all cron entries and asks
`job.Matches(now)`, so cron jobs simply skip occurrences that fell during
the pause and resume firing on the next matching minute after Start.

## Guarantee scope: one process

The schedule and fired-occurrence state are held in this process's memory,
and no inter-process coordination exists: N processes of one application fire
each job N times, against shared data. This path is not request-driven. See
the framework README, "Concurrency and Deployment".
