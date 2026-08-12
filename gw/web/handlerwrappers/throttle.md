# Throttle Middleware

`Throttle{AppProvider, BucketGroupID, KeyProvider}` limits requests by a
caller-defined string key. Per request it reaches the throttle service through
its framework handle, extracts the key with `KeyProvider`, and asks
`Allow(BucketGroupID, key, now)`. On refusal it answers HTTP 429 with the
structured `RateLimited` error and does not call the inner handler.

The service is reached through its handle, so an un-admitted throttle service —
stopped by an operator, mid-teardown, or never wired — answers HTTP 503
`ServiceUnavailable`: a limiter that cannot compute a verdict does not pass
traffic. An endpoint carrying this middleware is reached through the throttling
service strictly; there is no bypass state.

## Key providers

`KeyProvider` decides what the limit counts: a session UID, a session id, an
address, a composite. A provider returning `ok=false` blocks the request —
failure to identify the bucket is never a pass.

`IPThrottleKey(appProvider)` is the shipped address-keyed provider: it resolves
the caller's address per the deployment's trusted-proxy declaration
(`trusted_proxy_cidrs`). With no declaration, every request arriving through a
proxy resolves to the proxy's address and shares one bucket.

## What an address does not identify

An address is not a principal, in three distinct ways. Each affects what a
per-address limit can be relied on for.

- **Many callers per address.** Carrier NAT, and any single-egress network,
  place large numbers of clients behind one address. A limit hit by one client
  applies to all of them.
- **Addresses are reassigned.** Where an ISP rotates addresses, a limited
  client can reconnect to a fresh address, and an unrelated client can inherit
  a limited one.
- **Addresses are cheap in IPv6.** A client with a delegated prefix can rotate
  through many source addresses, so per-address counters do not accumulate
  against it.

## Consequence for how to use address keying

Per-address throttling is a volumetric control — it bounds traffic from one
network path. It is not an identity control: it cannot bound attempts against a
single account, because the attempts can move between addresses and unrelated
callers can share one. Where a request carries an authenticated identity, key
the limiter on that identity instead, and use the per-address limit only as the
outer bound.
