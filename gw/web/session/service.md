# Session Service

A **host** for the per-protocol session managers (cookie, bearer, future
flavors). Owns the shared state those managers coordinate against —
the `SessionLocks` store and the `KVDB` reference — and runs nothing in
the background: the lock store manages its own memory (entries exist only
while held or waited for; see `g/locking`), so there is nothing left for
this service to tick over.

This service is not independent infrastructure that does its own work —
it's a container that becomes useful only when at least one session
manager is attached, plus the admission gate the session middleware
checks per request.

What it holds:

- `SessionLocks` (`locking.Store`) — the per-session critical-section
  lock store, shared with every attached manager. Which implementation
  fills the seat is the app's, named at `Prepare`.
- The `KVDB` reference the managers read and write sessions through.
- One manager field per session protocol (e.g. `CookieSessionManager`,
  `BearerSessionManager`). Each manager is a **concrete type specific to
  its protocol** — no shared interface. All of this is available from the
  moment `Prepare` returns and stays available for the lifetime of the
  process.

## Prepare

`Core.PrepareSessionService(sessionLocks)`:
- Requires `MainKVDB` set, and a non-nil lock store — the framework picks
  neither.
- Constructs the Service (`NewService`) and registers it with
  `Core.RegisterService`, which places it in the composition graph the
  start and terminate walks follow.

After `Prepare`, the app attaches the session managers for whichever
protocols it uses:
- `Core.PrepareCookieSessions(...)` → wires `CookieSessionManager`
- `Core.PrepareBearerSessions(...)` → wires `BearerSessionManager`

An app uses one, the other, or both. Unwired fields stay `nil` and the
middleware paths for those protocols are simply never reached.

## Start() / Stop()

Pure state transitions — there is no background work to begin or halt.
The states still matter: they are what the admission gate answers with,
so an operator's `svc stop` makes the session middleware refuse, exactly
as for any other service. The session *data* is independent of this
lifecycle: KVDB rows keep expiring on their TTLs, and the lock store
keeps draining itself, whatever state the service is in.

## Terminate()

Terminal: state stays `TERMINATING`; the service cannot be `Start`ed
again in this process lifetime. Fires the framework's `Terminated`
channel so `Core.WaitServicesTerminated` can count this service as done.
There is no stop activity to wait for, so it completes immediately and
always cleanly.

No in-process resources to release; the lock store and the session
managers are reclaimed by GC at process exit. KVDB-side cleanup remains
the KVDB's responsibility (TTL).

## Operator note

If you need to drain or invalidate sessions, do it through the manager
APIs (e.g. `CookieSessionManager.DestroyUserSession` or by deleting the
relevant KVDB keys). Stopping the service gates the middleware; it has no
effect on session validity or session-row presence.
