# Throttle LimiterInMemBuckets

Token-bucket rate limiter. Apps register `BucketGroup`s at boot (each
defining burst + refill rate); middleware calls `TryAdmit(ctx, groupID,
bucketID, now)` per request.

`LimiterInMemBuckets` is one implementation of `Limiter`, the interface consumers hold.
Its counters live in this process's memory, so it computes a verdict for
every call and its error is always nil.

This service consists of 2 things:

1. **Throttle bucket system** — `groups` map + `TryAdmit()`/`GetBucket()`/
   `SetBucketGroup()`/`Inspect()`. The data and the request-path API.
   `TryAdmit` creates a bucket on first sight of an id, atomically
   (`LoadOrStore`), so concurrent first requests share one bucket rather
   than each getting a full burst.
2. **Background cleanup service** — a goroutine that periodically prunes
   stale buckets.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 is available from the moment `Prepare` returns and stays available for
the lifetime of the process.

## Prepare

`Core.PrepareThrottleService(cleanupCycle, cleanupOlderThan)`:
- Constructs the LimiterInMemBuckets (`NewLimiterInMemBuckets`) with the given cleanup-cycle and
  idle-bucket-expiry config.
- Initialises the `groups map[string]*BucketGroup` (empty).
- Registers the LimiterInMemBuckets with `Core.RegisterService`, which places it in the
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
- `TryAdmit()` continues to throttle requests using whatever the `groups`
  map currently holds.
- Existing `*Bucket`s continue to fill/drain at their configured rates.
- New `*Bucket`s are still created lazily by `TryAdmit()` on first hit per
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

"Stop throttle" ≠ "pause the limiter." Stop halts background cleanup; the
buckets stay and `TryAdmit` keeps computing honest verdicts, because they are
passive state and no lifecycle answer is folded into the rate verdict. Whether
this limiter may be used at all is decided in front of the pointer, by the
framework handle a consumer holds it through (`Framework Service Composition
Graph` §11.1).

So a stopped limiter keeps refusing what is over its limit; what stops is the
sweeping of idle buckets, which then live until Start resumes it.

## Guarantee scope: one process

Buckets are held in this process's memory, so the enforced rate is per
process: N processes of one application admit N times the configured rate.
See the framework README, "Concurrency and Deployment".
