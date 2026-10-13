package pubsubs

import (
	"context"

	"github.com/x64c/gwf/gw/svc"
)

// Broker is the part in the middle of pub/sub: it keeps which subscriptions want which topics and hands each
// published message to them. Every broker is a service.
type Broker interface {
	svc.Service

	// Publish hands payload, published to topic, to every subscription on topic. It never waits for a subscriber.
	Publish(ctx context.Context, topic, payload string) error

	// Subscribe creates a subscription to topics and returns once it receives every message published to them from
	// then on. ctx bounds the call and is the subscription's lifetime: canceling it ends the subscription.
	Subscribe(ctx context.Context, topics ...string) (*Subscription, error)

	// Stats returns a snapshot of the broker's subscriptions.
	Stats() Stats
}
