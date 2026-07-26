# Session Service

A **host** for the per-protocol session managers (cookie, bearer, future
flavors). Owns the shared state those managers coordinate against
(`SessionLocks` + `KVDB` reference) and runs a background cleanup
goroutine over that shared state.

This service is not independent infrastructure that does its own work —
it's a container that becomes useful only when at least one session
manager is attached. With no managers, nothing acquires `SessionLocks`,
the lock map stays empty, and the cleanup goroutine ticks over nothing.

This service consists of 2 things:

1. **Shared session state + per-protocol manager fields** —
   `SessionLocks` (the per-session critical-section lock store), the
   `KVDB` reference, and one manager field per session protocol
   (e.g. `CookieSessionManager` for the cookie protocol,
   `BearerSessionManager` for the bearer protocol). Each manager is a
   **concrete type specific to its protocol** — no shared interface.
2. **Background SessionLocks cleanup service** — a goroutine that
   periodically walks `SessionLocks` and deletes entries whose
   corresponding KVDB session row has already expired.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 is available from the moment `Prepare` returns and stays available for
the lifetime of the process.

## Prepare

`Core.PrepareSessionService(kvdb, cleanupCycle, cleanupOlderThan)`:
- Constructs the Service (`NewService`).
- Initialises an empty `SessionLocks` store (`lockstore.New()`).
- Stores the `KVDB` reference (used at cleanup time for existence checks).
- Registers the Service with `Core.AddService` so the lifecycle loop
  manages it.

After `Prepare`, the app attaches the session managers for whichever
protocols it uses:
- `Core.PrepareCookieSessions(...)` → wires `CookieSessionManager`
- `Core.PrepareBearerSessions(...)` → wires `BearerSessionManager`

An app uses one, the other, or both. Unwired fields stay `nil` and the
middleware paths for those protocols are simply never reached.

## Start()

Spawns the background cleanup goroutine — a ticker at `cleanupCycle`.
Each tick:
- Walks `SessionLocks` entries.
- For each entry idle ≥ `cleanupOlderThan`, asks KVDB whether the
  underlying session key still exists.
- Deletes the `SessionLocks` entry if its KVDB key is gone (TTL-expired
  in Redis).

The age filter keeps the work proportional to *likely-stale* entries
rather than the total entry count.

## Stop()

Halts the cleanup goroutine. The session data plane keeps working in
full:

- `SessionLocks.Acquire` / `Release` continue to serve per-request
  session locking.
- `BearerSessionManager` — create / destroy / extend / fetch sessions all
  function normally.
- `CookieSessionManager` — same.
- KVDB session rows are independent of this service's lifecycle: Redis
  TTL keeps expiring rows on schedule whether the cleanup goroutine is
  running or not.

What stops: the in-process **pruning** of stale `SessionLocks` entries.
A session that expires in KVDB during a Stop window leaves its
`SessionLocks` entry behind; the entry stays in the map until `Start()`
resumes the cleanup goroutine.

That's a **memory effect** — the `SessionLocks` map grows by one entry
per session that expired during the pause and never got pruned. Per-entry
cost is small (a `*LockEntry` struct), but a long Stop window with high
session churn accumulates.

`Start()` again resumes pruning; the next few cycles catch up by deleting
entries whose KVDB keys are now gone.

## Terminate()

Same as `Stop()` for this service — halts the cleanup goroutine — plus:
- Terminal: state stays `TERMINATING`. The service cannot be `Start`ed
  again in this process lifetime.
- Fires the framework's `Terminated` channel so
  `Core.WaitServicesTerminated` can count this service as done.

No in-process resources to release; the `SessionLocks` store and the
session managers are reclaimed by GC at process exit. KVDB-side cleanup
remains Redis's responsibility (TTL).

## Operator note

"Stop session" pauses **only** background `SessionLocks` pruning. All
session-handling — login, cookie / bearer auth middleware, per-session
locks, session create / destroy / extend through the managers —
continues to serve requests normally.

If you need to drain or invalidate sessions, do it through the manager
APIs (e.g. `CookieSessionManager.DestroyUserSession` or by deleting the
relevant KVDB keys). Stopping the service has no effect on session
validity or session-row presence.
