# Throttle Service

Token-bucket rate limiter. Apps register `BucketGroup`s at boot (each
defining burst + refill rate); middleware calls `Allow(groupID, bucketID,
now)` per request.

This service consists of 2 things:

1. **Throttle bucket system** — `groups` map + `Allow()`/`GetBucket()`/
   `SetBucketGroup()`/`Inspect()`. The data and the request-path API.
   `Allow` creates a bucket on first sight of an id, atomically
   (`LoadOrStore`), so concurrent first requests share one bucket rather
   than each getting a full burst.
2. **Background cleanup service** — a goroutine that periodically prunes
   stale buckets.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 is available from the moment `Prepare` returns and stays available for
the lifetime of the process.

## Prepare

`Core.PrepareThrottleService(cleanupCycle, cleanupOlderThan)`:
- Constructs the Service (`NewService`) with the given cleanup-cycle and
  idle-bucket-expiry config.
- Initialises the `groups map[string]*BucketGroup` (empty).
- Registers the Service with `Core.RegisterService`, which places it in the
  composition graph the start and terminate walks follow.

After `Prepare`, the app registers concrete bucket groups via
`SetBucketGroup(id, conf)` — this *must* happen before `Start` (Start
enforces it: `SetBucketGroup` after `Start` calls `log.Fatalf`).

Everything from this section is independent of `Start`/`Stop`/`Terminate`
state. The configuration and the empty data structures are in place from
the moment Prepare returns.

## Start()

ONE background goroutine: the **cleanup ticker**. Every `cleanupCycle`, it
walks the `groups` map and prunes individual `*Bucket` entries that have
been idle ≥ `cleanupOlderThan`. That goroutine is the *entire* lifecycle
surface — there is nothing else the service runs in the background.

## Stop()

Just that cleanup goroutine.

The data plane keeps working:
- `Allow()` continues to throttle requests using whatever the `groups`
  map currently holds.
- Existing `*Bucket`s continue to fill/drain at their configured rates.
- New `*Bucket`s are still created lazily by `Allow()` on first hit per
  bucketID.

The only observable change while stopped: stale buckets stop being
pruned. The map grows over time as new bucketIDs appear and old ones
linger. Memory creeps up; throttle correctness is unaffected.

`Start()` again resumes cleanup; all bucket state survived the pause.

## Terminate()

Mechanically equivalent to `Stop()` for this service — halts the cleanup
goroutine, leaves the `groups` map in memory.

Distinctions from `Stop`:
- Terminal: state stays `TERMINATING`. The service cannot be `Start`ed
  again in this process lifetime.
- Fires the framework's `Terminated` channel so
  `Core.WaitServicesTerminated` can count this service as done.

No throttle-specific resources to release — the `groups` map is reclaimed
by GC at process exit.

## Operator note

"Stop throttle" ≠ "pause the limiter." Stop halts background cleanup, and
the data plane keeps its buckets — but `Allow` opens with
`state != RUNNING → return true`, so a stopped service **admits
everything**. Stopping it does not preserve the limit; it removes it.

That fail-open verdict is a defect, not a feature: `Allow` returns a bare
`bool`, which cannot express "I cannot evaluate this", so it answers
wrongly by construction whichever way it picks. The fix is for callers to
be gated in front of the service rather than for the service to judge its
own availability — see the handle-acquisition question in
`Framework Service Composition Graph` §11.1. Until that lands, treat
`svc stop throttle` as disabling rate limiting.

## Guarantee scope: one process

Buckets are held in this process's memory, so the enforced rate is per
process: N processes of one application admit N times the configured rate.
See the framework README, "Concurrency and Deployment".
