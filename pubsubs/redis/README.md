# pubsubs/redis

`BrokerWithLocalSubscriptions`: a `pubsubs.Broker` built on **Redis pub/sub** (`PUBLISH` / `SUBSCRIBE`) and a
`pubsubs.LocalSubscriptionManager`. A publish reaches every process subscribed to its topic on the same Redis — every
instance of the app, and any other program, in any language, that uses the same topic string.

## Participants and material

| | what it is | where it lives |
|---|---|---|
| Redis pub/sub | matches a channel (= a topic, the same string) to the connections subscribed to it, and carries each message to them; keeps nothing | the Redis server |
| subscriber connection | ONE per process, subscribed to every topic this process has local subscriptions on | this process ↔ Redis |
| publishing client | ordinary connections (a pool) for `PUBLISH`; a subscribed connection can run no other command | this process ↔ Redis |
| local subscriptions | the subscriptions of this process — queues, end and miss notices, the five ends | `pubsubs.LocalSubscriptionManager`, in memory |
| the loop | one goroutine: decides every `SUBSCRIBE` / `UNSUBSCRIBE`, hands each received message to the local subscriptions | this process |

```
publish(T, p) ──PUBLISH T p──▶ Redis ──▶ every process's subscriber connection subscribed to T (this one too)
                                              │
                                    the loop: Deliver(T, p)
                                              ▼
                               local subscriptions on T ──▶ their message queues
```

## Why one connection per process, not one per subscription

Redis sees connections, never what runs inside a process. Serving each subscription from Redis directly would take a
connection each — about 22 KB in Redis and 64 KB of client buffers apiece (go-redis reads and writes through 32 KiB
buffers), under Redis's client limit (`maxclients`, 10 000 by default) shared with everyone. One connection per process
carries all its topics; the local subscription manager splits that one stream by topic.

## The loop

- **Subscribing.** The manager reports each topic's latest subscription count. A topic going above 0 is subscribed on
  the connection; one going to 0 is unsubscribed — so the connection holds exactly the topics this process needs.
  All of it is decided in the one loop, from the latest counts. A topic has at most one SUBSCRIBE awaiting Redis's
  confirmation; one going to 0 meanwhile is unsubscribed once the confirmation is in. So each confirmation answers the
  latest SUBSCRIBE of its topic, and a second one can only mean a restored connection.
- **`Subscribe` waits.** It returns only once Redis has confirmed every one of its topics — a message published
  before then would not reach this process, and the subscription would miss it with no notice. A subscription that
  ends first (its context canceled, or its queue filled by messages of its topics already confirmed) is not returned.
- **Holes and the miss notice.** When the connection breaks, go-redis opens a new one and subscribes again to its
  topics. It sees the break when reading from or writing to the connection fails; after 3 s with nothing from Redis it
  sends a PING, so a quiet connection is written to as well. What was published in between never reached this
  process. Redis confirming a topic already confirmed is how the loop sees this happened; once the connection is
  subscribed to every topic again, every local subscription gets a miss notice.
  The same notice follows a Start after a Stop: nothing is received while stopped.
- **Slow subscribers.** A local subscription whose queue is full is ended by the manager, as with every broker. The
  loop never waits on a subscriber, so the connection keeps up with Redis — which disconnects a subscriber connection
  that falls too far behind (`client-output-buffer-limit pubsub`).

## Topics and isolation

Topics and payloads reach Redis exactly as given. Redis keeps pub/sub channels per server, not per database, so two
apps on one Redis that use the same topic string meet on it; a topic string that should stay an app's own carries
something only that app uses.
