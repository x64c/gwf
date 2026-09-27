# passwd — stored passwords

Satellite `auth/passwd`: how a password is kept and checked — never the password itself, only
an encoding made from it. Where encodings are stored, how a name finds its account, and which
session a login opens are the app's.

## Encodings name their algorithm

A `passwd.Hasher` is one algorithm. Everything it writes is self-describing: the algorithm's id
leads, then its parameters, the salt and the hash —

```
$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>     PHC string format
$2b$12$<salt+hash>                               modular crypt format (bcrypt)
```

— the same standard `/etc/shadow` and the mainstream password libraries use. Verifying runs the
algorithm again on the offered password with the encoding's parameters and salt, and compares
in constant time; nothing is ever decoded back into a password.

## A group: one current, any accepted

An app holds its hashers as a `passwd.HasherGroup`:

- **Hash** — by the current hasher only.
- **Verify** — by whichever hasher of the group owns the encoding's id; an id none owns is refused.
- **rehash** — reported by a successful Verify when the owner is not the current hasher, or the
  current hasher's parameters changed. The app then stores `Hash` of the password it just
  verified — the one moment it has it — in place of the old encoding.
- **IsCurrent** — the same judgment without a password, to count what is left to migrate.
- **VerifyUnknown** — the work of a Verify for a name the app does not know, so response time does
  not reveal which names exist.

Normally the group has no accepted hasher. To change algorithm: make the new hasher current and
keep the old one accepted; every login on an old encoding comes out rehashed, so their count
only falls; at zero, drop the old hasher. Raising the current algorithm's parameters needs no
group change — its encodings at other parameters report rehash the same way.

## The method

`passwd.Verifier` is the password authentication method: a name and a password in, an
`authn.VerifiedIdentity{Method: passwd.Method, Subject: <the account's canonical name>}` out.
Storage stays the app's — two callbacks:

- `passwd.LookupFunc` — name → subject and stored encoding, or not found.
- `passwd.StoreRehashFunc` — replace a subject's encoding, only while it is still the old one.

An unknown name and a wrong password get the same work (`VerifyUnknown`) and the same answer,
`errs.InvalidCredentials`. Every login's hashing runs in one of at most `maxConcurrentHashes`
slots; one that gets no slot before its request ends answers `errs.PasswordCheckBusy`. A rehash
that fails to store is logged, never a failed login. Mapping the subject to a local user and
opening a session are the next steps, as for every method.

## The login handlers

One per session flavor; each reads a form post or a JSON body (`passwd.NameField`,
`passwd.PasswordField`) — the input's format is the client's, the session's flavor the app's:

| handler | opens | wire it behind |
|---|---|---|
| `passwd.CookieLoginHandler` | a cookie session, then `cookie.FinishLogin` (303 to the intended URI or `SuccessPath`); refusals a JSON error, or a 303 to `FailurePath` | `CheckOriginHeader` (a login form has no session yet to carry a CSRF token), a body limit, the throttles |
| `passwd.BearerLoginHandler` | a bearer session for the `Client-Id`'s client (a client under `""` serves callers that send none), answering `security.AuthResponseBody` with the app's own ID token | a body limit, the throttles |

`passwd.NameThrottleKey(normalize)` keys `handlerwrappers.ThrottleFixedGroup` by the name a login
carries, normalized by the app's own rule — so one account's guesses are bounded however many
addresses they come from, and an unknown name is limited exactly like a real one.

## Implementations

Named by their material.

| hasher | encoding | conf |
|---|---|---|
| `passwd.Argon2id` (RFC 9106) | PHC, `$argon2id$v=19$m=…,t=…,p=…$<salt>$<hash>` | `passwd.Argon2idConf` — no defaults; `NewArgon2id` refuses a conf below OWASP's minimum configurations, a salt under 16 bytes or a hash under 32 |
