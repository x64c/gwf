# Web Service

HTTP server wrapper around `net/http`. The app supplies a listen address
and an `http.Handler` (usually the router); the service spins up an
`*http.Server` on Start, drains gracefully on Stop, and rebuilds the
server fresh on the next Start cycle.

Unlike `throttle` / `session` / `jobsched`, this service has **no
lifecycle-independent data plane**. There is no API surface that does
useful work without an active server — the registered handler is just
configuration waiting to be served. So `Stop` here means *the HTTP
endpoint goes offline*; `Start` brings it back online.

This service consists of 2 things:

1. **HTTP configuration** — `addr` + `handler`, preserved across cycles
   on the Service struct. Survives `Stop`/`Start`/`Terminate`; not
   reconstructed unless the Service itself is recreated.
2. **The HTTP server** — the `*http.Server` instance, the port binding,
   the connection accept loop inside `Serve`, and the per-
   request goroutines stdlib spawns. This is what the lifecycle
   controls.

The framework lifecycle (`Start`/`Stop`/`Terminate`) controls **only #2**.
#1 stays put for the lifetime of the Service value.

## Prepare

`Core.PrepareWebService(conf, buildHandler, deps...)`:
- Validates `conf` (its own invariants, then `drain_timeout_secs` <
  `svc_terminate_timeout_secs`).
- Registers the Service with `Core.RegisterServiceFunc`: the web service's
  own `MayUse` (throttle, session — when prepared) plus `deps` are resolved,
  then `buildHandler(d)` builds the HTTP handler from those `Deps`, and the
  Service (`NewServiceAs("WebService", handler, conf)`) is placed in the
  composition graph the start and terminate walks follow.
- Handlers hold no graph edges: a service a handler uses is declared in
  `deps`, and the handler's gated handle comes from `d.Handle`. The web
  service then starts after that service and is torn down before it. A
  handler that uses no service ignores the `Deps`.

The `*http.Server` itself is **not** built at Prepare time — it's built
in `Start` (and rebuilt each cycle, because `http.Server` is one-shot
after `Shutdown`).

## Start()

- Builds a fresh `*http.Server` via `ServerConf.newHTTPServer` (timeouts and
  `BaseContext` come from conf). (Mandatory rebuild — the previous one is
  dead after `Shutdown`.)
- Makes a fresh **drain notice** for the cycle and stores its receive end in
  every request context, as a value. A handler gets it with
  `DrainNoticeReceiverFromContext(r.Context())`.
- **Binds the listener itself**: `net.Listen("tcp", addr)`. A bind failure
  cancels both per-cycle contexts and is returned from `Start`, so the
  service never reports RUNNING on a port it does not hold.
- Only then sets RUNNING and spawns the run goroutine, which:
  - Starts an inner goroutine running `s.Server.Serve(s.listener)` — not
    `ListenAndServe`, since the listener is already bound. It returns
    `http.ErrServerClosed` on graceful drain (mapped to nil), otherwise a
    serving error.
  - Itself sits on a `select` watching for either `<-s.Ctx.Done()` (the
    normal Stop path) or `<-serverErr` (server died on its own).

`Start` fails **synchronously** on a bind failure — the listener is verified
before RUNNING is reported. Only *serving* failures remain asynchronous,
arriving on `serverErr`.

## Stop()

Cancels the per-cycle ctx, which triggers the graceful-drain path inside
the run goroutine:

0. Closes the cycle's drain notice — the drain starts. It cancels nothing:
   ordinary handlers keep running and finish inside the drain window. A
   long-lived handler (a stream) that selects on the notice finishes now,
   so it does not hold `Shutdown` until the drain deadline.
1. `s.Server.SetKeepAlivesEnabled(false)` — idle keep-alive conns get
   closed at next request, so they don't hold up the drain.
2. `s.Server.Shutdown(gracefulCtx)` — stdlib stops accepting new conns,
   waits for in-flight handlers to return, then closes everything.
3. Wait for `<-serverErr` — the `Serve` goroutine returns
   `http.ErrServerClosed` (mapped to nil) on graceful drain.

After Stop:
- No new requests are accepted (port released).
- In-flight handlers ran to completion (or were cut off by the drain
  deadline).
- The `*http.Server` instance is dead and cannot be reused — `Start`
  rebuilds it.

**Two timeouts to keep distinct:**
- `Stop(ctx)` — the caller's deadline for how long Stop will *wait* for
  the run goroutine to actually exit. If `ctx` expires first, Stop
  returns its error, but the drain keeps running in the background until
  the next budget expires.
- **The conf-driven internal drain budget** for `Server.Shutdown` — the
  ceiling on how long stdlib waits for in-flight handlers. It comes from
  `ServerConf.DrainTimeoutSecs` (`conf.drainTimeout()`), not from Stop's
  ctx, and is validated at boot to be shorter than the service's terminate
  timeout. Long-running request handlers can be force-cancelled by this
  deadline regardless of what ctx the operator gave to Stop.

**Two signals a handler can see:**

| signal | when | how a handler gets it | effect |
|---|---|---|---|
| drain notice | the drain **starts** | `DrainNoticeReceiverFromContext(r.Context())` — a channel that is closed | none by itself; a handler that selects on it may finish now |
| request context canceled | the drain window **closes** | `r.Context().Done()` | in-flight work is canceled and must unwind |

If the server died on its own (a serving error — a port conflict would have
failed `Start`) instead of
being asked to stop, the run goroutine's `<-serverErr` branch fires and
the goroutine exits without going through the drain path. The state
field stays at whatever it was (likely `RUNNING`) because the deferred
`transitionAfterRun()` only flips `STOPPING → READY`, not `RUNNING →
anything`. That's a known limitation — operators should watch the log
for `[ERROR]` lines from this service if `State()` reports `RUNNING` but
the port isn't responding.

## Terminate()

Same flow as `Stop()` for this service (cancel + graceful drain + wait),
plus:
- Terminal: state stays `TERMINATING`. The service cannot be `Start`ed
  again in this process lifetime.
- Fires the framework's `Terminated` channel so
  `Core.WaitServicesTerminated` can count this service as done.

The `addr` and `handler` references stay on the struct until process
exit; nothing actively releases them.

## Operator note

"Stop web" = **the HTTP endpoint is offline**. The bound port is
released, in-flight requests are drained on a 15-second internal budget,
and new connections are refused until `Start()` rebinds.

There is no graceful "soft pause" — the listener closes and the port
goes free. If you want to keep the process alive while the HTTP surface
is offline (e.g., for a maintenance window where you do operator work
via UDS), `Stop` on this service is the right verb.
