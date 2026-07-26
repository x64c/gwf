# Body Limit Middleware

## Edge layer vs application layer

A web-server/reverse-proxy layer (nginx, Caddy, Apache, HAProxy, ALB, ingress
controllers, etc.) typically caps request body size vhost-wide (e.g. nginx
`client_max_body_size`, with a default of 1 MB). The framework's `BodyLimit`
middleware is the **application-layer complement** that provides:

- **Per-endpoint precision.** Edge layers usually apply one cap per vhost; an
  endpoint accepting JSON tokens should be capped at a few hundred bytes while
  an upload endpoint may need megabytes. Per-route caps live in the app.
- **Security in code.** A cap declared in route registration survives
  infrastructure changes (container rebuilds, K8s ingress switches, ops
  migrations). Edge-only config can drift silently.
- **Coverage of any path that bypasses the edge.** Local debugging,
  internal service-to-service calls, or future direct connections all
  benefit from caps being enforced in code, not just at the edge.

The edge layer's cap should be set to the largest legitimate per-endpoint
limit plus a margin. Each route then sets its own tighter cap.

## Two-layer enforcement inside the middleware

The middleware itself has two layers:

1. **Fast-path: Content-Length header check.** If the incoming request
   declares (via `Content-Length`) more bytes than the configured `Max`,
   reject upfront with `413 ContentLengthTooLarge`. No body bytes read.

2. **Source-of-truth: `http.MaxBytesReader` wrap.** The body reader is
   wrapped so any read past `Max` returns an error during streaming.
   Catches:
   - Chunked-encoded bodies (carry no Content-Length).
   - Clients lying about Content-Length (sending more bytes than declared).
   - Bodies from custom socket-level senders that don't follow the Fetch
     spec or any HTTP library — only the wire byte count is truthful.

Honest browser JS, the Go standard `http.Client`, and most libraries set
Content-Length correctly and would be caught by the fast-path. Layer 2
exists for the cases where the declaration is missing or wrong; that's
where the security guarantee actually lives.

### Why the streaming counter doesn't hurt performance

Counting happens **per Read call**, not per byte. The HTTP runtime reads
bodies in chunks (typical buffer sizes 4 KB–64 KB). For a 1 GB body, that's
~30,000 Read calls each doing one integer add + one compare. Negligible
against JSON parsing / network I/O costs.

The counter never trusts the Content-Length header value; only the actual
bytes read advance it. This is what makes the limit honest against
clients lying about declared size.

## Error sentinels

The middleware uses two distinct sentinels to let ops/telemetry distinguish
the two paths:

- `errs.ContentLengthTooLarge` — fast-path. Client declared oversize via
  Content-Length. Detail string includes the limit and the declared value,
  e.g. `"max 1024 < got 34535 bytes"`.
- `errs.RequestBodyTooLarge` — source-of-truth. `MaxBytesReader` triggered
  mid-stream. **Returned by the handler**, not the middleware, since the
  handler is the one that observes the read error from
  `json.UnmarshalRead` / `io.ReadAll`.

Handlers can choose to:
- Inspect `*http.MaxBytesError` via `errors.As` and respond with the
  precise `RequestBodyTooLarge` sentinel, or
- Treat the failure as a generic `JSONUnmarshalFailed.WithCause(err)`. The
  detail string in the cause still carries enough info for debugging; the
  difference is only the typed error code returned to the client.

## Per-endpoint guidance

| Endpoint shape | Apply BodyLimit? | Typical cap |
|---|---|---|
| Endpoints that don't read a body | No | — |
| Endpoints reading small credential / token JSON | Yes | tight (a few KB) |
| Endpoints reading typical app-data JSON | Yes | modest (tens of KB) |
| File upload endpoints | Yes | per-feature size cap |
| Webhook receivers | Yes | provider-dependent (often around 1 MB) |

Endpoints handling credential / token JSON in particular should be tight —
they're a tempting DoS target if uncapped. The legitimate body shape is
small and known at design time.

## Wiring example

```go
smallJSONBodyLimit := &handlerwrappers.BodyLimit{Max: 1024}

router.Handle("POST <path>",
    &someHandler{...},
    smallJSONBodyLimit,        // outermost — caps body bytes before anything reads them
    &handlerwrappers.SomeAuthCheck{...},  // runs after body cap, before handler
)
```

A single `BodyLimit` instance can be reused across multiple routes that
share the same cap. Routes needing a different cap construct their own
instance with a different `Max`.

## Ordering with other middlewares

The variadic wrapper list is applied outer-to-inner left-to-right
(`router.Handle(p, h, w1, w2)` builds `w1(w2(handler))`). `BodyLimit`
should be **earlier** in the list than any middleware that may consume
the body, so the cap is enforced before the body is read downstream.
See `routing/README.md` ("Handler Wrapper Order") for the full rule.
