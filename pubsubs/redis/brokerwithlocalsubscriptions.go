package redis

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"log"
	"maps"
	"slices"
	"sync"

	lowimpl "github.com/redis/go-redis/v9"
	"github.com/x64c/gwf/gw/pubsubs"
	"github.com/x64c/gwf/gw/svc"
)

var _ pubsubs.Broker = (*BrokerWithLocalSubscriptions)(nil)

var errNotRunning = errors.New("pubsubs/redis: broker not running")

// BrokerWithLocalSubscriptions is a broker built on Redis pub/sub and a pubsubs.LocalSubscriptionManager: Redis carries
// messages between processes, and each process's local subscriptions take them from there. See the README.
type BrokerWithLocalSubscriptions struct {
	name         string
	conf         pubsubs.Conf
	brokerConf   BrokerConf
	state        svc.AtomicState
	terminated   chan error // one-shot; fires when Terminate completes
	manager      *pubsubs.LocalSubscriptionManager
	waitRequests chan waitRequest

	mu      sync.Mutex      // guards the fields below
	client  *lowimpl.Client // publishes, and opens the subscriber connection; made by the first Start
	cancel  context.CancelFunc
	stopped chan struct{} // per cycle; closed once the loop has ended and the subscriber connection is closed
}

func NewBrokerWithLocalSubscriptions(conf pubsubs.Conf, brokerConf BrokerConf) (*BrokerWithLocalSubscriptions, error) {
	return NewBrokerWithLocalSubscriptionsAs("PubSubService", conf, brokerConf)
}

// NewBrokerWithLocalSubscriptionsAs is NewBrokerWithLocalSubscriptions with the service name given explicitly.
func NewBrokerWithLocalSubscriptionsAs(name string, conf pubsubs.Conf, brokerConf BrokerConf) (*BrokerWithLocalSubscriptions, error) {
	if err := brokerConf.Validate(); err != nil {
		return nil, err
	}
	manager, err := pubsubs.NewLocalSubscriptionManager(conf, pubsubs.LocalSubscriptionManagerOptions{TopicSubscriptionCountUpdates: true})
	if err != nil {
		return nil, err
	}
	b := &BrokerWithLocalSubscriptions{
		name:         name,
		conf:         conf,
		brokerConf:   brokerConf,
		terminated:   make(chan error, 1),
		manager:      manager,
		waitRequests: make(chan waitRequest),
	}
	b.state.Store(svc.StateREADY)
	return b, nil
}

func (b *BrokerWithLocalSubscriptions) Name() string {
	return b.name
}

func (b *BrokerWithLocalSubscriptions) State() svc.State {
	return b.state.Load()
}

// Start : READY → RUNNING. Connects to Redis (within init_timeout_secs), opens the subscriber connection and starts
// the loop; returns only once both connections answered. After a Stop, the loop subscribes again to every topic that
// still has local subscriptions, and once Redis confirms them all, puts a miss notice on every subscription: nothing
// was received while stopped.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (b *BrokerWithLocalSubscriptions) Start(parentCtx context.Context) error {
	if b.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if b.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", b.state.Load())
	}
	log.Printf("[INFO][%s] Starting.", b.Name())
	b.mu.Lock()
	if b.client == nil {
		b.client = lowimpl.NewClient(&lowimpl.Options{Addr: b.brokerConf.addr(), Password: b.brokerConf.PW})
	}
	client := b.client
	b.mu.Unlock()

	initCtx, cancelInit := context.WithTimeout(parentCtx, b.brokerConf.initTimeout())
	defer cancelInit()
	if err := client.Ping(initCtx).Err(); err != nil {
		return fmt.Errorf("redis %s: %w", b.brokerConf.addr(), err)
	}
	ps := client.Subscribe(initCtx)
	if err := ps.Ping(initCtx); err != nil {
		_ = ps.Close()
		return fmt.Errorf("redis %s subscriber connection: %w", b.brokerConf.addr(), err)
	}
	received := ps.ChannelWithSubscriptions()

	// What the counts are now, taken before RUNNING lets Subscribe in: only subscriptions kept across a Stop.
	_ = b.manager.TakeTopicSubscriptionCountUpdates()
	kept := b.manager.Stats().SubscriptionCountPerTopic

	ctx, cancel := context.WithCancel(parentCtx)
	stopped := make(chan struct{})
	b.mu.Lock()
	b.cancel, b.stopped = cancel, stopped
	b.mu.Unlock()
	go b.run(ctx, ps, received, kept, stopped)
	b.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (broker: Redis pub/sub at %s, one subscriber connection + local subscriptions; queue capacity %d)", b.Name(), b.brokerConf.addr(), b.conf.QueueCapacity)
	return nil
}

// Stop : RUNNING → STOPPING → READY. A pause: the loop ends and the subscriber connection closes, so nothing is
// received; the local subscriptions are kept. ctx governs only how long Stop waits.
func (b *BrokerWithLocalSubscriptions) Stop(ctx context.Context) error {
	if b.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if b.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", b.state.Load())
	}
	b.state.Store(svc.StateSTOPPING)
	if err := b.stop(ctx); err != nil {
		return err
	}
	b.state.Store(svc.StateREADY)
	return nil
}

// Terminate : any → TERMINATING (irreversible). Stops if running, ends every subscription (EndByTermination) and
// closes the Redis client. Fires Terminated on EVERY exit path (deferred send).
func (b *BrokerWithLocalSubscriptions) Terminate(ctx context.Context) (err error) {
	if b.state.Load() == svc.StateTERMINATING {
		return nil // idempotent — returns before the defer arms
	}
	prevState := b.state.Load()
	b.state.Store(svc.StateTERMINATING)
	log.Printf("[INFO][%s] Terminating.", b.Name())
	defer func() {
		b.terminated <- err // THE ONLY send site; unconditional, exactly once
		if err == nil {
			log.Printf("[INFO][%s] Terminated.", b.Name())
		} else {
			log.Printf("[ERROR][%s] Terminated with stop error: %v", b.Name(), err)
		}
	}()
	if prevState == svc.StateRUNNING || prevState == svc.StateSTOPPING {
		err = b.stop(ctx)
	}
	b.manager.EndAll()
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client != nil {
		_ = client.Close()
	}
	return err
}

func (b *BrokerWithLocalSubscriptions) Terminated() <-chan error {
	return b.terminated
}

func (b *BrokerWithLocalSubscriptions) stop(ctx context.Context) error {
	log.Printf("[INFO][%s] Stopping.", b.Name())
	b.mu.Lock()
	cancel, stopped := b.cancel, b.stopped
	b.mu.Unlock()
	if cancel == nil {
		return nil
	}
	cancel()
	select {
	case <-stopped:
		log.Printf("[INFO][%s] Stopped.", b.Name())
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop deadline exceeded: %w", ctx.Err())
	}
}

// Publish sends payload to topic through Redis (PUBLISH, both passed as given): every process subscribed to topic
// there receives it — this one included, through its own subscriber connection.
func (b *BrokerWithLocalSubscriptions) Publish(ctx context.Context, topic, payload string) error {
	if topic == "" {
		return errors.New("pubsubs/redis: publish names an empty topic")
	}
	b.mu.Lock()
	client := b.client
	b.mu.Unlock()
	if client == nil {
		return errNotRunning
	}
	return client.Publish(ctx, topic, payload).Err()
}

// Subscribe creates a subscription to topics and returns once Redis has confirmed every one of them on this
// process's subscriber connection — from then on, every message published to them reaches it. ctx bounds the wait and
// is the subscription's lifetime. A subscription that ends before then (its context canceled, or its queue filled by
// messages of its topics already confirmed) is not returned. Refused while the broker is not running.
func (b *BrokerWithLocalSubscriptions) Subscribe(ctx context.Context, topics ...string) (*pubsubs.Subscription, error) {
	if b.state.Load() != svc.StateRUNNING {
		return nil, errNotRunning
	}
	b.mu.Lock()
	stopped := b.stopped
	b.mu.Unlock()
	sub, err := b.manager.Subscribe(ctx, topics...)
	if err != nil {
		return nil, err
	}
	req := waitRequest{topics: topics, endNotice: sub.EndNoticeReceiver(), reply: make(chan struct{}, 1)}
	requests := b.waitRequests
	for {
		select {
		case requests <- req:
			requests = nil // sent; the answer comes on req.reply
		case <-req.reply:
			return sub, nil
		case <-sub.EndNoticeReceiver():
			if sub.EndReason() == pubsubs.EndByContextCancel {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("pubsubs/redis: subscription ended before Redis confirmed its topics: %v", sub.EndReason())
		case <-stopped:
			sub.Unsubscribe()
			return nil, errNotRunning
		}
	}
}

func (b *BrokerWithLocalSubscriptions) Stats() pubsubs.Stats {
	return b.manager.Stats()
}

// run is the broker's one loop: every SUBSCRIBE and UNSUBSCRIBE is decided here, from the latest topic subscription
// counts — never from the order the counts changed in. A topic has at most one SUBSCRIBE whose confirmation is not in
// yet: one no longer wanted meanwhile is unsubscribed once its confirmation is in. So every confirmation answers the
// latest SUBSCRIBE of its topic, and a second one for a confirmed topic can only come from go-redis subscribing again
// on a restored connection. The loop also hands every message Redis sends to the local subscriptions, answers
// Subscribe's waits, and puts a miss notice on every subscription once a hole on the connection has closed. kept is
// the counts of the subscriptions kept across a Stop: they missed everything published while nothing was received.
func (b *BrokerWithLocalSubscriptions) run(ctx context.Context, ps *lowimpl.PubSub, received <-chan interface{}, kept map[string]int, stopped chan struct{}) {
	defer close(stopped)
	defer func() { _ = ps.Close() }()

	counts := make(map[string]int)        // each topic's latest subscription count; a topic at 0 is left out
	topics := make(map[string]topicState) // the topics on the connection: SUBSCRIBE sent, no UNSUBSCRIBE since
	var waiters []waitRequest

	// apply sends what the latest counts call for on the given topics, one SUBSCRIBE and one UNSUBSCRIBE at most. An
	// UNSUBSCRIBE takes its topic off the connection at once, as go-redis does: its confirmation tells nothing.
	apply := func(changed iter.Seq[string]) {
		var subscribe, unsubscribe []string
		for t := range changed {
			switch st := topics[t]; {
			case counts[t] > 0 && st == 0:
				subscribe = append(subscribe, t)
				topics[t] = topicSubscribing
			case counts[t] == 0 && st == topicSubscribed:
				unsubscribe = append(unsubscribe, t)
				delete(topics, t)
			}
		}
		if len(subscribe) > 0 {
			if err := ps.Subscribe(ctx, subscribe...); err != nil && ctx.Err() == nil {
				log.Printf("[WARN][%s] SUBSCRIBE %d topic(s): %v (sent again when the connection is restored)", b.Name(), len(subscribe), err)
			}
		}
		if len(unsubscribe) > 0 {
			if err := ps.Unsubscribe(ctx, unsubscribe...); err != nil && ctx.Err() == nil {
				log.Printf("[WARN][%s] UNSUBSCRIBE %d topic(s): %v", b.Name(), len(unsubscribe), err)
			}
		}
	}
	takeCounts := func() {
		updates := b.manager.TakeTopicSubscriptionCountUpdates()
		for t, n := range updates {
			if n > 0 {
				counts[t] = n
			} else {
				delete(counts, t)
			}
		}
		apply(maps.Keys(updates))
	}
	confirmed := func(req waitRequest) bool {
		for _, t := range req.topics {
			if topics[t] != topicSubscribed {
				return false
			}
		}
		return true
	}
	// answerWaiters answers every wait whose topics are all confirmed and drops every wait whose subscription has
	// ended: its Subscribe has returned.
	answerWaiters := func() {
		kept := waiters[:0]
		for _, req := range waiters {
			select {
			case <-req.endNotice:
				continue
			default:
			}
			if confirmed(req) {
				req.reply <- struct{}{}
			} else {
				kept = append(kept, req)
			}
		}
		clear(waiters[len(kept):])
		waiters = kept
	}

	maps.Copy(counts, kept)
	missNoticeDue := len(kept) > 0
	apply(maps.Keys(kept))

	for {
		select {
		case <-ctx.Done():
			return
		case <-b.manager.TopicSubscriptionCountUpdatesNoticeReceiver():
			takeCounts()
			answerWaiters()
		case req := <-b.waitRequests:
			takeCounts() // the subscribe behind this request is counted by now
			waiters = append(waiters, req)
			answerWaiters()
		case m, more := <-received:
			if !more {
				return
			}
			switch v := m.(type) {
			case *lowimpl.Message:
				b.manager.Deliver(v.Channel, v.Payload)
			case *lowimpl.Subscription:
				if v.Kind != "subscribe" {
					continue
				}
				switch topics[v.Channel] {
				case topicSubscribing:
					topics[v.Channel] = topicSubscribed
					if counts[v.Channel] == 0 { // no longer wanted while its confirmation was on the way
						apply(slices.Values([]string{v.Channel}))
					}
				case topicSubscribed:
					// A second confirmation: the connection was restored and go-redis subscribed again. What was
					// published in between never reached this process.
					missNoticeDue = true
				}
				if missNoticeDue && v.Count >= len(topics) {
					b.manager.PutMissNoticeToAll()
					missNoticeDue = false
				}
				answerWaiters()
			}
		}
	}
}
