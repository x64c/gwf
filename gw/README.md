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

On a request path, a panic is answered where a recovery is installed — a
`routing.Router` that recovers in its own `ServeHTTP`, a `Wrap` that recovers,
or `handlerwrappers.RecoverPanic` applied to a handler or a router. Reaching
none, it passes to net/http, which logs it and closes the connection.

On a lifecycle path, a panic escaping a service's `Start` or `Terminate` is
recovered by the framework, which completes the ordered rollback or teardown
and then raises the panic again on the goroutine that called `Run` — a
`defer`/`recover` there catches it; uncaught, the process dies by panic. A
panicking scheduled job fails that job, with the panic as the error handed to
its callbacks. A panicking UDS command handler ends that connection with an
error answer. Inside goroutines a service spawns itself, only the service's
own code can recover a panic; report it as the service's failure — the error
from `Start`, or the error on the `Terminated` send.

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

### Coordination state and the mode

An app states its coordination mode in `.core.json` — `"coord_mode"`:
`"inproc"` or `"crossproc"`, never a default — and each `Prepare*` seats the
implementation the mode calls for. Under `inproc` the state below lives in
this process; under `crossproc` it lives in the app's shared KVDB, under the
app's name, where every process of the app contends for it.

| Component | State | `inproc` | `crossproc` |
|---|---|---|---|
| `throttle` | token buckets | this process | shared KVDB |
| `locking` — action locks, session locks (`max_sessions_per_user`) | lock ledger | this process | shared KVDB |
| `jobsched` — `OncePerApp` jobs | fired-occurrence marks | this process (the one instance) | shared KVDB: each occurrence fires once per app |
| `jobsched` — `OncePerInstance` jobs | schedule | this process | each process |
| `jwtassert` replay cache | admitted assertion ids | this process | shared KVDB |
| JWKS | private keys on local disk; active key id in the KVDB | per host | per host |
| `pubsubs` | subscriptions | the seated broker's reach | the seated broker's reach |
| `web/sse` | open streams | this process: a stream is one client's connection | this process |
| `uds` | admin socket at the conf's `socket_path` | this process | each process, at its own path |

The pub/sub broker is the app's choice, not the mode's:
`BrokerLocalSubscriptionsOnly` reaches the subscriptions of the publishing
process; `pubsubs/redis` reaches every process subscribed on the same Redis.

### Multiple processes of distinct applications

Cross-process consistency for shared rows is provided by the store (SQL
transactions and row locks); each application's coordination state concerns
only its own work.

### Multiple processes of one application

Supported under `crossproc`: limits, locks, the session cap, once-per-app jobs
and the replay cache hold across every process of the app. What stays per
process or per host:

- JWKS rotation publishes the new key id to the shared KVDB but writes the
  private key to local disk. Processes on other hosts then reference a key id
  they cannot sign with, and `jwks-delete-old-rsakeys` removes keys still in
  use elsewhere. [ToDo] Support CrossProc mode.
- Each process needs its own admin socket (`socket_path`): a process whose
  path is already served refuses to start.
- `OncePerInstance` jobs fire in every process — that is what they are.
- `BrokerLocalSubscriptionsOnly` delivers only within the publishing process.

Under `inproc`, a second process of the same app splits all of that state
between two processes that cannot see each other, and no error is returned:
limits admit N times the rate, locks exclude only within a process, and every
scheduled job fires once per process. One instance is the deployment's promise
to keep. This applies regardless of host count and of request distribution:
each request is served by exactly one process, so routing is not the
constraint.

### Partitioned deployments

One process per fully separate state set — own database, own KVDB, own users.
Nothing is shared, so nothing requires coordination.

### `AppName`

Partition key for shared-store keys, and — with the mode — the app's identity:
processes running under `crossproc` with the same name against the same shared
store are one app and share its coordination state; a different name is a
different app, whatever the binary. Distinct apps sharing a KVDB therefore
need distinct names: the name keeps their keys apart.

### Classifying application state

State describing one process — its connections, its service states — is
unaffected by process count. State asserted as global while held in process
memory — a limit, a lock, a schedule — holds only while one process runs.

# Debug Code vs Non-Debug Code
`//go:build debug && verbose` vs `//go:build !(debug && verbose)`
