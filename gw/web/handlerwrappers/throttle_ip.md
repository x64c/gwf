# IP Throttle Middleware

Keys a throttle bucket by the caller's resolved address: `ThrottleIP{AppProvider,
BucketGroupID}` calls `ClientIPResolver.ClientIP(r)`, then
`ThrottleService.Allow(BucketGroupID, ip, now)`. On refusal it answers HTTP 429
and does not call the inner handler; the response message currently includes the
resolved address.

The resolved address depends on the deployment's trusted-proxy declaration
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

## Consequence for how to use it

Per-address throttling is a volumetric control — it bounds traffic from one
network path. It is not an identity control: it cannot bound attempts against a
single account, because the attempts can move between addresses and unrelated
callers can share one. Where a request carries an authenticated identity, key
the limiter on that identity instead, and use the per-address limit only as the
outer bound.
