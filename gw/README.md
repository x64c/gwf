# gw Framework

## Design Philosophy
- **Compile-time over runtime** — prefer typed structs, generics, and compile-time checks over runtime type assertions, switches, or reflection when possible.
- **Report, don't exit** — ending the process belongs to `main`; see below.

## Reporting Failure: response, error, panic

Guidance for code written with this framework, and the intent its own packages
are written toward.

Three channels, chosen by **who receives the report**.

| Channel | Recipient | Use it for |
|---|---|---|
| HTTP response | the client | anything the system understood and decided: 404, 401, 403, 429, 413, 503 |
| `error` | a Go caller that can decide | the environment misbehaving: unreachable store, missing or malformed config, a failed dependency |
| `panic` | nobody | a promise the program itself made has been broken |

**A response is not a failure.** Rejecting a request is an outcome. A wrapper
that refuses writes its status and returns without calling the handler it
wraps; nothing further is reported.

**An error needs a caller who can act on it** — abort boot, retry, fall back,
degrade. Where a caller exists, an error is always the right channel, because
it leaves the decision with the code that owns the consequences.

**A panic is not confusion.** Confusion about the outside world is an error.
Panic asserts that an invariant this code itself guarantees has been violated —
a nil where the type says otherwise, a `switch` branch proven unreachable, an
index just bounds-checked. Nothing can "handle" it, because handling would mean
continuing to compute wrong answers.

### Wiring mistakes

Configuration and wiring errors — a required field unset, a component used
before it was prepared, a duplicate registration — are none of the three
categories above. They are author errors, detectable before the application
serves anything. The guidance:

> **Report them as an error wherever a channel exists. Panic only where the API
> shape genuinely has none** — a package-level `var`, or a signature that cannot
> carry one.

That is what the `…OrPanic` constructors are for, and why they are rare: each
one exists solely because its value is declared once, at package level, where
nothing can receive an error. The error-returning constructor is always the
primary form.

### `log.Fatal` and `os.Exit`

**Ending the process is the application's decision, not a library's.** Both
`log.Fatal*` and `os.Exit` skip every deferred function, so a library that
exits can leave the host's resources unreleased; neither can be intercepted by
an embedding host; and a code path that exits cannot be tested. Library code
should report — an error where a caller exists, a panic where none can — and
leave the exit to `main`.

In an application's `main`, `log.Fatal` is appropriate in two places:

- **during boot**, when a `Prepare`/`Load` step returns an error — nothing is
  serving, no listener is bound, no pool is open, so exiting immediately costs
  nothing;
- **after shutdown has completed**, when `Run` returns — the ordered teardown
  has already run, so nothing is skipped.

It is not appropriate **while services are running**. There, cancel the root
context and let the ordered shutdown finish, then exit on what it reports.
Exiting mid-flight bypasses the teardown this framework exists to sequence.

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
