package pubsubs

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/x64c/gwf/gw/svc"
)

var _ Broker = (*BrokerLocalSubscriptionsOnly)(nil)

// BrokerLocalSubscriptionsOnly is a broker built on a LocalSubscriptionManager and nothing else: a published message
// reaches the subscriptions of this process only. It cannot miss a message without ending the subscription, so its
// subscriptions never get a miss notice.
type BrokerLocalSubscriptionsOnly struct {
	name       string
	state      svc.AtomicState
	terminated chan error // one-shot; fires when Terminate completes
	manager    *LocalSubscriptionManager
}

func NewBrokerLocalSubscriptionsOnly(conf Conf) (*BrokerLocalSubscriptionsOnly, error) {
	return NewBrokerLocalSubscriptionsOnlyAs("PubSubService", conf)
}

// NewBrokerLocalSubscriptionsOnlyAs is NewBrokerLocalSubscriptionsOnly with the service name given explicitly.
func NewBrokerLocalSubscriptionsOnlyAs(name string, conf Conf) (*BrokerLocalSubscriptionsOnly, error) {
	manager, err := NewLocalSubscriptionManager(conf, LocalSubscriptionManagerOptions{})
	if err != nil {
		return nil, err
	}
	b := &BrokerLocalSubscriptionsOnly{
		name:       name,
		terminated: make(chan error, 1),
		manager:    manager,
	}
	b.state.Store(svc.StateREADY)
	return b, nil
}

func (b *BrokerLocalSubscriptionsOnly) Name() string {
	return b.name
}

func (b *BrokerLocalSubscriptionsOnly) State() svc.State {
	return b.state.Load()
}

// Start : READY → RUNNING. It has no goroutine of its own, so nothing in it can fail.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (b *BrokerLocalSubscriptionsOnly) Start(context.Context) error {
	if b.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if b.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", b.state.Load())
	}
	b.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (broker: local subscriptions only — reaches this process only; queue capacity %d)", b.Name(), b.manager.conf.QueueCapacity)
	return nil
}

// Stop : RUNNING → STOPPING → READY. A pause: the subscriptions are kept.
func (b *BrokerLocalSubscriptionsOnly) Stop(context.Context) error {
	if b.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if b.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", b.state.Load())
	}
	b.state.Store(svc.StateSTOPPING)
	log.Printf("[INFO][%s] Stopping.", b.Name())
	b.state.Store(svc.StateREADY)
	log.Printf("[INFO][%s] Stopped.", b.Name())
	return nil
}

// Terminate : any → TERMINATING (irreversible). Ends every subscription (EndByTermination); fires Terminated on every
// exit path (deferred send).
func (b *BrokerLocalSubscriptionsOnly) Terminate(context.Context) (err error) {
	if b.state.Load() == svc.StateTERMINATING {
		return nil // idempotent — returns before the defer arms
	}
	b.state.Store(svc.StateTERMINATING)
	log.Printf("[INFO][%s] Terminating.", b.Name())
	defer func() {
		b.terminated <- err // THE ONLY send site; unconditional, exactly once
		log.Printf("[INFO][%s] Terminated.", b.Name())
	}()
	b.manager.EndAll()
	return nil
}

func (b *BrokerLocalSubscriptionsOnly) Terminated() <-chan error {
	return b.terminated
}

func (b *BrokerLocalSubscriptionsOnly) Publish(_ context.Context, topic, payload string) error {
	if topic == "" {
		return errors.New("pubsubs: publish names an empty topic")
	}
	b.manager.Deliver(topic, payload)
	return nil
}

func (b *BrokerLocalSubscriptionsOnly) Subscribe(ctx context.Context, topics ...string) (*Subscription, error) {
	return b.manager.Subscribe(ctx, topics...)
}

func (b *BrokerLocalSubscriptionsOnly) Stats() Stats {
	return b.manager.Stats()
}
