# Throttle Service

Token-bucket rate limiter. Apps register `BucketGroup`s at boot (each
defining burst + refill rate); middleware calls `Allow(groupID, bucketID,
now)` per request.

This service consists of 2 things:

1. **Throttle bucket system** — `groups` map + `Allow()`/`GetBucket()`/
   `SetBucketGroup()`/`Inspect()`. The data and the request-path API.
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
- Registers the Service with `Core.AddService` so the lifecycle loop
  manages it.

After `Prepare`, the app registers concrete bucket groups via
`SetBucketGroup(id, conf)` — this *must* happen before `Start` (Start
enforces it: `SetBucketGroup` after `Start` panics).

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

"Stop throttle" ≠ "disable throttling." Stop only pauses background
cleanup; the request-path filter remains active. To truly disable rate
limiting, the middleware itself would need to short-circuit based on
service state — that is not the current contract.
