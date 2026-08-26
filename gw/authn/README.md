# Currently Supported Authentication Flows

Authentication in this framework is composed from parts: a method establishes
a `VerifiedIdentity`; the app maps it to a local user and opens a session of
the flavor it chose. No part mandates a composition. The flows below are the
compositions the parts have been exercised in.

## Terms

| Term | Meaning |
|---|---|
| **IdP** | Identity provider: the external authority that authenticates a human (an OIDC provider). |
| **Relying party** | The side that consumes the IdP's answer. It has two halves: **initiate** (builds the authorization URL, issues the flow ticket) and **verify** (exchanges the code, validates the ID token). One app may hold both halves, or two apps may split them. |
| **Auth server** | A framework app holding the verify half on behalf of other framework apps: it exchanges their codes with the IdP and issues them its own bearer tokens. Its identity provider is the IdP; to its callers, it is the identity provider. |
| **Upstream / downstream** | A framework app called by another framework app is that caller's upstream; the caller is its downstream. `fwupstream.Client` is the downstream's handle on one upstream. |
| **Machine client** | A caller that authenticates as a program, not a user: by a per-request signed assertion. |
| **Asserted user** | The user a machine client names inside its assertion, for whom it asks a token. |
| **Flow ticket** | `authn.FlowTicket`: the state, nonce, and PKCE verifier issued at initiate and consumed exactly once at return. |
| **Verified identity** | `authn.VerifiedIdentity`: an external identity, verified, not yet mapped to a local user. Every method produces one. |
| **Session flavor** | What the app opens for a mapped user: a **cookie session** (`session/cookie`, browser) or a **bearer session** (`session/bearer`, token-carrying client). Either way the session is a row in the KVDB; the cookie or token only names it. |
| **Gate** | A handler wrapper that admits a request by an existing session or client registration (`handlerwrappers.CookieUserSession`, `BearerClient`, `BearerSession`, `BearerUserSession`, …). |
| **Delegated Code Exchange** | Split relying party: the browser-facing app initiates; an auth server exchanges the code and issues bearer tokens. |
| **Direct Code Exchange** | Whole relying party: the app exchanges the code with the IdP itself. |
| **Machine Assertion** | A machine client authenticates each request by a self-signed, request-bound JWT. |
| **User Token by Machine Assertion** | A machine client presents an assertion naming a user; the upstream answers with a bearer token for that user. |
| **Session Authentication** | Request-time authentication by a session opened earlier: the request carries a session cookie or `Authorization: Bearer`, a gate looks the session row up in the KVDB and places the session data in the request context. |
| **Endpoint roles** | *Login*, *callback*, *verify*, *refresh*, *exchange*, *revocation* name what an endpoint does in a flow. Every path is the app's; the framework fixes none. Where a downstream must know an upstream's path, it comes from `fwupstream.ClientConf`. |

## Parts

| Package | Role |
|---|---|
| `authn` | The seam: `Method`, `VerifiedIdentity`, request-context carriage (`WithVerifiedIdentity` / `VerifiedIdentityFromContext`), and `FlowManager` / `FlowTicket` for browser-mediated flows. |
| `auth/oidc` | Relying-party halves for the OIDC authorization-code flow with PKCE. `Provider.AuthCodeURL` (initiate), `Provider.VerifyAuthCode` (verify). A `Provider` is described entirely by configuration. |
| `auth/fwauthserver` | Verify half delegated to an auth server: `Verifier.VerifyAuthCode` forwards the code and flow secrets, validates the auth server's ID token against its JWKS. |
| `auth/jwtassert` | Machine authentication: `Signer` (client half), `Verifier` (receiving half). |
| `security` | Wire bodies between a downstream and an auth server: `AuthRequestBody`, `AuthResponseBody`, `RefreshAccessTokenRequestBody`. |
| `session/cookie`, `session/bearer` | Session flavors; session rows live in the KVDB. Each row carries per-upstream token slots. |
| `fwupstream` | Downstream side of an upstream relationship: client configuration, bearer-carrying request ladder, access-token refresh, upstream JWKS fetch. |
| `handlerwrappers` | Gates by session flavor and by bearer client registration. |

## Flow ticket

Every browser-mediated flow opens with `FlowManager.IssueTicket` and closes
with `FlowManager.ConsumeTicket`.

- Cookie `__Host-authn-flow`, `Secure`, `HttpOnly`, `SameSite=Lax`, max age
  600 s, sealed under a cipher context bound to the app name and the cookie
  name.
- The ticket's state rides the authorization request and is compared in
  constant time on return; the nonce is echoed inside the ID token; the PKCE
  verifier is proven at code exchange (`PKCEChallengeS256` for the request).
- `ConsumeTicket` clears the cookie on every outcome. A return request
  presented twice finds no ticket. Every failure is `errs.InvalidFlowTicket`.

## Delegated Code Exchange

The initiate half runs in the browser-facing app; the verify half runs in an
auth server. The browser-facing app never holds the IdP client secret.

```
browser ── GET login endpoint ──────────▶ app          IssueTicket; redirect to Provider.AuthCodeURL
browser ◀─ 302 ─────────────────────────  app
browser ── credentials ─────────────────▶ IdP
browser ◀─ 302 callback?code&state ─────  IdP
browser ── GET callback endpoint ───────▶ app          ConsumeTicket(state)
app     ── POST verify endpoint ────────▶ auth server  AuthRequestBody {auth_client_id, code, redirect_uri, nonce, pkce_verifier}
                                          auth server  Provider.VerifyAuthCode with the IdP; map identity → user;
                                                       bearer.SessionManager.CreateSession; sign id_token with own JWKS key
app     ◀─ 200 AuthResponseBody ────────  auth server  {access_token, refresh_token, expires_in, token_type, id_token}
app                                                    fwauthserver.Verifier validates id_token; map Subject → user;
                                                       cookie session; store the token pair on the session row
browser ◀─ Set-Cookie; 302 ─────────────  app
```

| Side | Parts |
|---|---|
| Browser-facing app | `authn.FlowManager` · `oidc.Provider` (initiate half; `ClientSecret` empty) · `oidc.AuthCodeRequestHandler` (login endpoint) · `fwauthserver.DelegatedExchangeCallbackHandler` (callback endpoint: ticket → `fwauthserver.Verifier` → `authn.UIDStrResolver` → cookie session + token pair → `cookie.FinishLogin`) · `cookie.SessionManager` |
| Auth server | `oidc.DelegatedExchangeAuthCodeVerifyHandler` (verify endpoint: caller by `Client-Id` → its `oidc.Provider` → `authn.UIDStrResolver` → bearer session → ID token signed with the active JWKS key, `Core.SignIDToken`) · `oidc.Provider` (verify half; holds the client secret) · `bearer.SessionManager` (a session group per client kind; clients registered by name → opaque id) · `bearer.RefreshAccessTokenHandler` · JWKS |

Configuration on the browser-facing side: `fwupstream.ClientConf` — `host`,
`client_id` (the browser-facing app's id at the auth server), `verify_external_auth_code`
(verify endpoint path per IdP id), `refresh_access_token`, `jwks_url`.

Session lifetime after login: the app calls the auth server as its user
through `UserSessionData.UpstreamRequestWithBearerRetriable`; on 401 the
ladder refreshes the pair at `refresh_access_token` (serialized per session
row) and retries once. A refresh sideloader
(`cookie.SetUserRefreshSideloader`) adds app-defined fields to the refresh
body.

Failures at verify: `*fwauthserver.UpstreamError` (the auth server answered
non-200; the body is carried whole), `errs.IDTokenInvalid`,
`errs.IDPUnavailable`.

## Direct Code Exchange

Both halves run in one app. The app holds the IdP client secret and resolves
the verified identity against its own user store.

```
browser ── GET login endpoint ──────────▶ app   IssueTicket; redirect to Provider.AuthCodeURL
browser ── credentials ─────────────────▶ IdP
browser ◀─ 302 callback?code&state ─────  IdP
browser ── GET callback endpoint ───────▶ app   ConsumeTicket(state); Provider.VerifyAuthCode(code, redirect_uri, nonce, pkce_verifier)
                                                map Subject/claims → user; cookie session
browser ◀─ Set-Cookie; 302 ─────────────  app
```

Parts: `authn.FlowManager` · `oidc.Provider` (both halves) ·
`oidc.AuthCodeRequestHandler` (login endpoint) ·
`oidc.DirectExchangeCallbackHandler` (callback endpoint: ticket → verify →
`authn.UIDStrResolver` → cookie session → `cookie.FinishLogin`) ·
`cookie.SessionManager`.

`authn.UIDStrResolver` is the app's: it maps the verified identity to the
uid string the session stores, or refuses (answered 401 with its error).
Which claim identifies the person, where users live, and what "may log in"
means are decided there. `cookie.FinishLogin` sets the session cookie and
redirects to the saved intended URI or the handler's `SuccessPath`.

`VerifyAuthCode` validates: RSA signature via the provider's JWKS, `exp`,
`aud` = `ClientID`, `iss` = `Issuer`, nonce echo, `RequiredClaims` equality,
`RequireEmailVerified` when set. `Subject` is the token's `sub`; the email
claim is data. Failures: `errs.AuthCodeExchangeFailed`,
`errs.IDTokenInvalid`, `errs.IDPUnavailable`.

An app that also serves other apps as an auth server (the auth-server side
of Delegated Code Exchange) runs this same verify half on their behalf.

## Machine Assertion

A program authenticates each request by a self-signed, request-bound JWT.
No session, no shared secret, no token issued.

```
client   Signer.Sign(method, target, body, extra) → compact JWS
client ── Authorization: JWTAssert <JWS> ──▶ app   Verifier.VerifyRequest(r) → VerifiedIdentity, *Client
```

- Claims: `iss` = `sub` = client id, `aud` = the verifying side, `iat`,
  `exp`, `jti`, `htm` (method), `htu` (path plus raw query), `body_hash`
  (base64url SHA-256, when a body is present). `extra` claims pass through
  to `VerifiedIdentity.Claims`. RS256.
- Verifying side configuration: clients keyed by name, each with `id`,
  `audience`, `public_key_dir` (`<kid>_public.pem`; the stem is the kid),
  `max_age`, `clock_skew`, `max_body_bytes`.
- Replay: `jti` is remembered until `exp`; a second presentation inside the
  window is `errs.AssertionReplayed`.
- Failures: `errs.AssertionNotFound`, `errs.InvalidAssertion` (detail says
  which check), `errs.AssertionReplayed`, `errs.AssertionClientUnknown`.

Gate: `jwtassert.Gate{Verifier}` — a handler wrapper; refuses with 401 and
the Verifier's sentinel (413 `RequestBodyTooLarge`), or places the identity
in the request context (`authn.VerifiedIdentityFromContext`) and calls the
wrapped handler.

## User Token by Machine Assertion

A downstream that owns its own users (Direct Code Exchange, or any local
login) uses an upstream as its user. The downstream proves itself as a
machine client and names the user in the assertion; the upstream issues a
bearer session for that user.

```
downstream   MachineSigner.SignRequest(POST, exchange endpoint, nil, {<user_claim>: uid})
downstream ── Authorization: JWTAssert ──▶ upstream   VerifyRequest; read the user claim;
                                                      bearer.SessionManager.CreateSession(group, client, uid)
downstream ◀─ 200 {access_token, expires_in, token_type} ─ upstream
downstream   token stored on its own session row (slot keyed by the upstream client)
downstream ── Authorization: Bearer ─────▶ upstream   BearerUserSession gate; the user's request
```

| Side | Parts |
|---|---|
| Downstream | `jwtassert.Signer` as the client's `fwupstream.MachineSigner` · `fwupstream.ClientConf` (`host`, `client_id`, `token_exchange`, `token_revoke`, `user_claim`, token cipher) · `UserSessionData.UpstreamRequestWithExchangeRetriable` (cached-or-exchanged bearer, retry once on 401) · `UserSessionData.UpstreamForgetExchanged` (logout: revoke + drop) |
| Upstream | `jwtassert.Verifier` behind `jwtassert.Gate` · `bearer.UserTokenExchangeHandler` (exchange endpoint) · `bearer.TokenRevokeHandler` (revocation endpoint) · `bearer.SessionManager` (a user-bound session group for exchanged tokens; the downstream registered under the same id in both the bearer and the jwtassert registries) · `handlerwrappers.BearerUserSession` |

- The exchange and revocation handlers are `bearer`'s; their paths, the
  user claim name (`UserClaim`), and what a claim value must look like to
  name a user (`ParseUser`) are the app's. The handlers read the verified
  identity from the request context, so they work behind the gate of any
  method that places one.
- An exchanged token has no refresh token: the row carries only the
  access-token field for that upstream (`Hub.StoreAccessToken`), and on a
  cache miss or a 401 the ladder exchanges again
  (`fwupstream.RowRequestWithExchangeRetriable`, written once against
  `TokenRow` like the refresh ladder). The token is stored only while the
  row exists; `UpstreamForgetExchanged` removes both token fields.
- Revocation: the downstream presents the access token to the upstream's
  revocation endpoint; `TokenRevokeHandler` destroys the session if the
  presenting client owns it (`{"revoked": true|false}`, idempotent).
- An unreachable upstream, or its gateway answering 502/504 for it, is
  reported as `errs.UpstreamUnavailable` with the upstream's status; the
  downstream chooses its own answer to its users (the ladder never decides
  it).

## Session Authentication

Every flow above ends by opening a session; from then on each request is
authenticated by that session, not by the flow. The gates in
`handlerwrappers` do this. A gate refuses by writing its status and never
calling the handler it wraps; on admission it places the session data in the
request context.

| Gate | Reads | Admits when | Places in context | Refusals |
|---|---|---|---|---|
| `BearerUserSession[UID]` | `Authorization: Bearer <token>` | the token's hash resolves to a live, user-bound session row in the KVDB whose group is in `AllowedGroups` (empty = any configured group); `ParseUID` accepts the stored uid | `*bearer.UserSessionData[UID]` | 401 `AccessTokenNotFound`, `BearerSessionNotFound`; 403 `BearerSessionShapeMismatch` (userless session), `BearerSessionGroupNotAllowed` |
| `BearerClient` | `Client-Id` header | the id is in the bearer client registry (and its group in `AllowedGroups`) | nothing | 401 `BearerClientNotFound` |
| `BearerSession` | nothing | the bearer protocol is in service | nothing | 503 `SessionServiceUnavailable` |
| `CookieUserSession[UID]` | the user session cookie | the cookie decrypts, its session row in the KVDB is live (absolute or sliding expiry per `ExpireMode`), `ParseUID` accepts the stored uid | `*cookie.UserSessionData[UID]` | cookie cleared and 303 to `LoginPath` (`?endpoint=protected` when no cookie, `?session=expired`, `?session=invalid`); the intended URI is kept in a cookie for the post-login redirect |
| `CookieSession` | nothing | the cookie protocol is in service | nothing | 503 `SessionServiceUnavailable` |

- Bearer access tokens are opaque: the gate hashes the token and looks the
  row up in the KVDB; no claims are parsed at request time. What the session asserts
  (its binds — client, user — and its group) was fixed when it was created.
- `ParseUID` is the app's: the stored identity is a string; the app's parser
  turns it into its UID type and must reject any string that names no user,
  the empty string included.
- `BearerClient` is for endpoints reached before a token exists (login,
  verify, JWKS). A route behind `BearerUserSession` already has its client:
  a session exists only for a registered client.
- Bearer session lifetime: `bearer.RefreshAccessTokenHandler` (app-registered
  path) rotates the pair via `SessionManager.ExtendSession` on a posted
  refresh token. Cookie session lifetime: extended per request under
  sliding expiry, fixed under absolute.

## Composition matrix

| Identity established by | Session opened | Exercised in |
|---|---|---|
| `fwauthserver` (auth server verifies) | cookie session at the downstream; bearer session at the auth server | Delegated Code Exchange |
| `oidc` (app verifies) | cookie session | Direct Code Exchange; the auth-server side of Delegated Code Exchange |
| `jwtassert` | none (per request) | Machine Assertion |
| `jwtassert` + asserted user | bearer session at the upstream; token cached on the downstream's cookie session row | User Token by Machine Assertion |


## CURRENTLY UNSUPPORTED

- **OIDC discovery.** `oidc.Provider` takes `issuer`, `auth_url`,
  `token_url`, `jwks_url` from configuration; no `.well-known` document is
  fetched.
- **Non-RSA ID tokens.** `oidc` and `fwauthserver` verify RSA signatures
  only; `jwtassert` signs and verifies RS256 only.
- **Multi-process assertion replay protection.** The `jwtassert` replay
  window is in-process; a verifier running as several processes has no
  shared window.
