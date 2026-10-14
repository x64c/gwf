# cors

Cross-origin resource sharing (CORS): how an app lets pages from other origins call it. A browser keeps a page from
reading a response from another origin unless the response says the page's origin may; before a request a plain HTML
form could not send — another method, a header like `Authorization`, a JSON body — it first asks with a **preflight**
(`OPTIONS`, no body, no credentials) and sends the request only if the answer allows it. CORS is a permission to
browsers; it protects nothing on the server — who may act stays the job of authentication.

## Pieces

| | |
|---|---|
| `Confs` | `config/.web-cors.json`: each access control key's `Conf`; `LoadConfs(appRoot)` reads it, `Confs.Conf(key)` takes one |
| `Conf` | one key's deployment half: `allow_origins`, `access_control_max_age_secs` — both required |
| `Policy` | a policy for the routes it is applied to — one route or many groups: a `Conf` plus `AllowMethods` and `AllowHeaders`; a handler wrapper that lets its origins read their responses |
| `PreflightHandler(policy)` | the handler that answers the preflights for those routes |

## Configuration and code

The deployment owns which sites may call and how long browsers reuse a preflight answer; the code owns what its routes
and pages use — the methods the routes serve and the headers the pages send are the same in every deployment.

```json
{
  "foo": { "allow_origins": ["https://a.example", "https://b.example:8443"], "access_control_max_age_secs": 600 },
  "bar": { "allow_origins": ["*"], "access_control_max_age_secs": 7200 }
}
```

```go
confs, err := cors.LoadConfs(appRoot)
foo, err := confs.Conf("foo")

oneCORSPolicy := cors.Policy{
	Conf:         foo,
	AllowMethods: []string{http.MethodPost},
	AllowHeaders: []string{"Authorization", "Content-Type"},
}
preflight, err := cors.PreflightHandler(oneCORSPolicy)
router.Handle("OPTIONS /one/{subpath...}", preflight) // outside every authentication wrapper
router.Group("/one/", oneRoutes,
	oneCORSPolicy, // first, before authentication
	// … authentication and the group's other wrappers
)
```

The same `Policy` value feeds both pieces, so the preflight's answer and the responses name the same origins; one
policy may serve several groups.

An origin is written exactly as a browser sends it: `scheme://host[:port]`, lowercase, no path, no default port.
`["*"]` — alone — allows any origin. `AllowHeaders` lists what a page **may** send — a header it does not send is
fine; it must list every non-safelisted header a page does send, `Content-Type: application/json` included, whether
or not the server reads it.

## A preflight's answer

| request | answer |
|---|---|
| from an origin the policy allows, for an allowed method and headers | 204 + `Access-Control-Allow-Origin`, `-Allow-Methods`, `-Allow-Headers`, `-Max-Age` |
| from any other origin | 403 |
| for a method or a header the policy does not allow | 403, naming it |
| `OPTIONS` that is not a preflight | 400 |

GET, HEAD and POST are always allowed methods — a browser allows them whatever the answer lists. Every answer carries
`Vary`, so a cache keeps answers for different origins and requests apart. A browser reuses an answer for
`access_control_max_age_secs`, per origin and exact URL.

A preflight only lets the request be **sent**. For the page to **read** the response, that response must carry
`Access-Control-Allow-Origin` as well — the `Policy`'s job as a wrapper.

## A response

| request | the `Policy` adds |
|---|---|
| from an origin the policy allows | `Access-Control-Allow-Origin: <that origin>` + `Vary: Origin` |
| from any other origin, or with no `Origin` | `Vary: Origin` only — the browser keeps the response from the page |
| any request, under `["*"]` | `Access-Control-Allow-Origin: *` |

It sets the headers before calling the next handler, so every answer behind it carries them — an authentication
refusal and an error as much as a success; placed after authentication, a 401 would reach the page as a network
error. It never wraps the `ResponseWriter`, so a stream still flushes.

## Refused at boot

`LoadConfs`: a file that is not an object, an unknown member, an empty key, an entry without `allow_origins` or with
`"*"` beside an origin, an origin a browser would never send or listed twice, `access_control_max_age_secs` not above
0. `Confs.Conf`: a key the file does not have, named. `PreflightHandler`: the same `Conf` checks, no `AllowMethods`, a
method or header that is not a token (or `"*"`). `Wrap`: the same `Conf` checks.
