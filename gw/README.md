# gw Framework

## Design Philosophy
- **Compile-time over runtime** — prefer typed structs, generics, and compile-time checks over runtime type assertions, switches, or reflection when possible.

## Concurrency and Deployment

### Concurrency within one process

Each HTTP request is served on its own goroutine, and the coordination
primitives listed below are internally synchronized.

### Guarantee scope of coordination state

| Component | State | Scope |
|---|---|---|
| `throttle` | token buckets | per process |
| `namedlocks` | mutexes (consumer returns HTTP 409) | per process |
| session `lockstore` | mutexes for session-cap and refresh serialization | per process |
| `jobsched` | schedule and fired-occurrence state | per process |
| JWKS | private keys on local disk; active key id in shared KVDB | per process |
| `uds` | admin socket path derived from the app name | per host |

### Multiple processes of distinct applications

Cross-process consistency for shared rows is provided by the store (SQL
transactions and row locks); each application's coordination state concerns
only its own work.

### Partitioned deployments

One process per fully separate state set — own database, own KVDB, own users.
Nothing is shared, so nothing requires coordination.

### `AppName`

Partition key for shared-store keys and socket paths. It must be unique among
processes that share a KVDB. A distinct name prevents key collisions; it does
not make a second process of the same application safe, because duties
(scheduled jobs, limits, locks) still duplicate and session state splits.

### Classifying application state

State describing one process — its connections, its service states — is
unaffected by process count. State asserted as global while held in process
memory — a limit, a lock, a schedule — holds only while one process runs.

## CURRENTLY UNSUPPORTED

### Multiple processes of one application

No error is returned; the guarantees weaken silently:

- A configured rate limit is enforced per process: N processes admit N times
  the rate.
- `max_sessions_per_user` is enforced per process: each acquires its own
  session lock, then reads the shared list as under cap.
- Named locks exclude only within a process: a duplicate submission receives
  HTTP 409 from one process and proceeds in another.
- Every scheduled job fires once per process, against shared data. This path
  is not request-driven.
- JWKS rotation publishes the new key id to the shared KVDB but writes the
  private key to local disk. Other processes then reference a key id they
  cannot sign with, and `jwks-delete-old-rsakeys` removes keys still in use
  elsewhere.
- On one host, a starting process removes an existing admin socket at the
  same path and replaces it.

This applies regardless of host count and of request distribution: each
request is served by exactly one process, so routing is not the constraint.

# Debug Code vs Non-Debug Code
`//go:build debug && verbose` vs `//go:build !(debug && verbose)`
