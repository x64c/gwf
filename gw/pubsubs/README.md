# pubsubs

Publish/subscribe: a publisher sends a message to a topic; every subscription to that topic receives it; neither side
knows the other. A **broker** is the part in the middle. A topic is any non-empty string, passed as given.

## Pieces

| | |
|---|---|
| `Broker` | the interface: `Publish`, `Subscribe`, `Stats`; every broker is a service (`svc.Service`) |
| `Subscription` | what `Subscribe` returns: a message queue and two notice channels |
| `Message` | `Topic` + `Payload`, as published |
| `EndReason` | why a subscription ended |
| `Conf` | `config/.pubsubs.json`: `queue_capacity` — required |
| `BrokerLocalSubscriptionsOnly` | a broker that reaches the subscriptions of this process only |
| `LocalSubscriptionManager` | the subscriptions of one process; brokers are built on it |

## What every broker promises

- A message reaches each subscription that exists when it is published, at most once. Nothing is kept: a message
  published to a topic with no subscription reaches no one.
- `Publish` never waits for a subscriber.
- `Subscribe` returns once the new subscription receives every message published to its topics from then on.

## A subscription

```go
sub, err := broker.Subscribe(ctx, "foo", "bar")
for {
	select {
	case msg := <-sub.MessageQueueReceiver(): // handle msg
	case <-sub.MissNoticeReceiver():          // messages may have been missed
	case <-sub.EndNoticeReceiver():           // ended; sub.EndReason() says why
		return
	}
}
```

| channel | holds |
|---|---|
| message queue | up to `queue_capacity` messages, in publish order |
| end notice | closed once, when the subscription ends |
| miss notice | at most one notice: messages may have been missed while the subscription went on; a second miss before it is taken adds nothing |

A subscription ends in exactly five ways, each its `EndReason`:

| | when |
|---|---|
| `EndByUnsubscribe` | the subscriber calls `Unsubscribe` |
| `EndByContextCancel` | the context given to `Subscribe` is canceled |
| `EndByFullQueue` | a message comes while its queue is full — the subscriber was not taking; messages still queued are dropped |
| `EndByTermination` | the broker is terminated |
| `EndByAbandonment` | no code holds the `Subscription` any more — found by the garbage collector, so some time after |

Keep the `Subscription` itself, not only its channels, for as long as you take from it.

Stopping a broker is a pause: its subscriptions are kept.

## BrokerLocalSubscriptionsOnly

A `LocalSubscriptionManager` and nothing else. Its reach is this process: under `crossproc`, each instance's
subscriptions receive only that instance's publishes. It cannot miss a message without ending the subscription, so it
never puts a miss notice.

## LocalSubscriptionManager

The subscriptions of one process, for building brokers: `Subscribe`, `Deliver` (puts one message into every
subscription on a topic), `PutMissNoticeToAll`, `EndAll`, `Stats`.

Its topics are split over 16 lock stripes by hash, so work on one topic locks only that topic's stripe.

**Topic subscription count updates** — on only when `LocalSubscriptionManagerOptions.TopicSubscriptionCountUpdates`
is set. Each change to a topic's subscription count is kept as that topic's latest count, and a notice is put into a
channel that holds at most one. The owner waits on `TopicSubscriptionCountUpdatesNoticeReceiver` and calls
`TakeTopicSubscriptionCountUpdates`: every topic changed since the last take, with its count now; 0 means none is
left. A topic that changed several times between takes appears once.
