# Security Headers Middleware

## Edge layer vs application layer

A web-server/reverse-proxy layer (nginx, Caddy, Apache, HAProxy, ALB, ingress
controllers, etc.) typically sets baseline response security headers vhost-wide.
The framework's `SecurityHeaders` middleware is the **application-layer
complement** for routes that need stricter or different policies than the edge
baseline.

Reasons to use the app-layer middleware on top of an edge baseline:

- **Per-route precision.** Edge layers usually apply one policy per vhost. A
  Content-Security-Policy that fits a privileged-area page may be wrong for a
  public marketing page on the same vhost. App-layer adds per-route-group
  granularity.
- **Code visibility.** A security-relevant decision in route registration
  survives infrastructure changes (container rebuilds, K8s ingress switches,
  ops migrations). Edge-only config can drift silently.
- **Coverage of any path that bypasses the edge.** Local debugging, internal
  service-to-service calls, or future direct connections all benefit from
  headers being set in code, not just at the edge.

## Per-header guidance

| Header | Global-suited (edge) | Per-route-suited (app) |
|---|---|---|
| `X-Content-Type-Options: nosniff` | yes — set once, applies everywhere | rarely overridden |
| `Strict-Transport-Security` (HSTS) | yes — per-host concern | rarely overridden |
| `Referrer-Policy` | yes — broad policy | occasionally per-route |
| `Permissions-Policy` | mostly yes — broad permissions | occasional per-route |
| `Content-Security-Policy` | partial — base CSP at edge | **frequently per-route** (admin, OAuth callback, embeds, etc.) |
| `X-Frame-Options` / `frame-ancestors` | usually `DENY` globally | per-route overrides for embed-friendly routes |

So the typical layering:

- **Edge layer**: HSTS, X-Content-Type-Options, Referrer-Policy, Permissions-Policy, plus a conservative base CSP / X-Frame-Options.
- **App layer**: per-route-group overrides of CSP and/or X-Frame-Options where needed.

## Behavior

- Each field on `SecurityHeaders` is empty by default. An empty field means
  **"don't set that header"** — the edge layer's value (if any) passes through.
- Set headers run BEFORE the inner handler. They apply to all responses, both
  success paths and error paths (e.g., `WriteErrorJSON`).
- Apps wire the middleware per route group via the routing API's variadic
  `handlerWrappers` parameter.

## Wiring examples

JSON-API style (minimal — most security headers are irrelevant to JSON
clients; let the edge layer handle the baseline):

```go
apiHeaders := &handlerwrappers.SecurityHeaders{
    ContentTypeOptions: "nosniff",
    ReferrerPolicy:     "no-referrer",
}
router.Group("<prefix>", routesFn, apiHeaders)
```

Web-app style with stricter CSP per route group:

```go
webHeaders := &handlerwrappers.SecurityHeaders{
    ContentTypeOptions:    "nosniff",
    StrictTransport:       "max-age=63072000; includeSubDomains",
    ContentSecurityPolicy: "default-src 'self'; script-src 'self'; style-src 'self'",
    FrameOptions:          "DENY",
    ReferrerPolicy:        "strict-origin-when-cross-origin",
    PermissionsPolicy:     "camera=(), microphone=(), geolocation=()",
}
router.Group("<prefix>", routesFn, webHeaders)
```

## Ordering with other middlewares

The variadic wrapper list is applied outer-to-inner left-to-right
(`router.Handle(p, h, w1, w2)` builds `w1(w2(handler))`). `SecurityHeaders`
sets response headers before the inner handler runs and never short-circuits,
so its placement in the wrapper list rarely matters — but conventionally
keep cross-cutting response-decorating wrappers (this, logging, etc.) later
in the list, with pre-checks (body limit, auth, session) earlier. See
`routing/README.md` ("Handler Wrapper Order") for the full rule.
