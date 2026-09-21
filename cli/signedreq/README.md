# signedreq

One HTTP request to a framework app, signed as a `jwtassert` client.

```
signedreq [flags] METHOD TARGET
```

`TARGET` is the request target — path plus optional `?query`, exactly as the
server will see it — not a URL. The host comes from the client's `audience`
unless `-base` overrides it.

```sh
signedreq -conf ~/.config/signedreq/foo.json GET /svc/ping
signedreq -conf ~/.config/signedreq/foo.json -d '{"note":"bar"}' POST /svc/notes
signedreq -app /srv/foo -client bar -kid bar-20260101 -key /keys/bar_private.pem GET /svc/ping
```

## Why it signs and sends in one act

A `jwtassert` assertion is bound to the method, the request target and the body
hash (`htm` / `htu` / `body_hash`), and carries a fresh `jti` the server admits
once. It is not a bearer token: it cannot be minted ahead, cached, or moved to
another call. So the thing that builds it has to be the thing that sends it.

`-sign-only` prints the `Authorization` value for composing with another
client, and hands that constraint back to the caller — a value minted for
`GET /a` fails on any other request.

## Every value is an argument

Nothing about a site is built in. `-conf` is a convenience only: a JSON file
supplying the same flags under the same names, without the dash. **An explicit
flag always beats the file**, and unknown members are refused.

Every member, and the flag it stands for. All are optional in the file; what
is required is decided by Identity below, not by the file.

| member | flag | value |
|---|---|---|
| `app` | `-app` | app root holding `config/.web-authn-jwtassert.json` |
| `client` | `-client` | client name in that conf to sign as |
| `id` | `-id` | client id, instead of `app` + `client` |
| `aud` | `-aud` | audience, instead of `app` + `client` |
| `max_age` | `-max-age` | assertion lifetime in seconds, instead of `app` + `client` |
| `kid` | `-kid` | key id; the server knows the public half as `<kid>_public.pem` |
| `key` | `-key` | private key PEM path |
| `base` | `-base` | `scheme://host` to send to; default: the audience |
| `headers` | `-H` | extra `"Name: value"` request headers |

Two member names differ from their flag: `max_age` (flag `-max-age`) and
`headers` (flag `-H`, repeatable).

A file naming an app's own conf:

```json
{
  "app":    "/srv/foo",
  "client": "bar",
  "kid":    "bar-20260101",
  "key":    "/keys/bar_private.pem"
}
```

A file for an app whose conf this host cannot read:

```json
{
  "id":      "opaque-client-id",
  "aud":     "https://foo.example",
  "max_age": 60,
  "kid":     "bar-20260101",
  "key":     "/keys/bar_private.pem",
  "headers": ["X-Trace: bar"]
}
```

Headers are the exception to "flag beats file": `-H` values from both are kept,
since two headers are not in conflict.

The file names a private key, so give it the protection that key deserves —
the tool reads it as whatever user runs the command.

## Identity

One of two ways, never a mixture:

- **`-app` + `-client`** — reads `id`, `audience` and `max_age` from that app's
  own `config/.web-authn-jwtassert.json`, the same file its server loads, so
  those three cannot drift from what is enforced. Available only where the
  app's configuration is readable.
- **`-id` + `-aud` + `-max-age`** — states them directly, for an app whose
  configuration this host cannot read.

`-kid` and `-key` are always required. The server knows the public half as
`<kid>_public.pem` in the client's key directory; the private half belongs to
the host that signs and is read at each call.

## Output

Body on stdout, status and headers on stderr under `-i`, so a pipeline sees
only the body. Exit status is 0 only for 2xx; a refusal prints the server's
error document and exits 1.

## Scope

It signs with the same `jwtassert.Signer` an app uses for its own machine
calls, so it exercises that path rather than a parallel implementation — and
therefore shares it: a fault in the scheme common to both halves would pass
here. An independent implementation is the check for that.
