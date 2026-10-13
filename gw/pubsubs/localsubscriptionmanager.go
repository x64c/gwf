package pubsubs

import (
	"context"
	"errors"
	"hash/maphash"
	"runtime"
	"sync"
)

// shardCount is how many lock stripes a manager splits its topics over: work on one topic locks only its stripe.
const shardCount = 16

var errTerminated = errors.New("pubsubs: subscriptions have ended for good (terminated)")

// LocalSubscriptionManager manages the subscriptions of this process: it creates them, delivers messages into their
// queues, ends them, tells them of misses and, when turned on, reports each topic's subscription count as it changes.
// Brokers are built on it.
type LocalSubscriptionManager struct {
	conf          Conf
	seed          maphash.Seed
	shards        [shardCount]shard
	updatesNotice chan struct{} // capacity 1; nil when the updates are off

	allMu      sync.Mutex // guards the two below
	all        map[*managedSubscription]struct{}
	terminated bool // set by EndAll; Subscribe refuses from then on
}

func NewLocalSubscriptionManager(conf Conf, opts LocalSubscriptionManagerOptions) (*LocalSubscriptionManager, error) {
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	m := &LocalSubscriptionManager{
		conf: conf,
		seed: maphash.MakeSeed(),
		all:  make(map[*managedSubscription]struct{}),
	}
	for i := range m.shards {
		m.shards[i].topics = make(map[string]map[*managedSubscription]struct{})
		if opts.TopicSubscriptionCountUpdates {
			m.shards[i].updates = make(TopicSubscriptionCountUpdates)
		}
	}
	if opts.TopicSubscriptionCountUpdates {
		m.updatesNotice = make(chan struct{}, 1)
	}
	return m, nil
}

func (m *LocalSubscriptionManager) shardOf(topic string) *shard {
	return &m.shards[maphash.String(m.seed, topic)%shardCount]
}

// Subscribe creates a subscription to topics: repeats are dropped; none, or an empty one, is refused. ctx is the
// subscription's lifetime: canceling it ends the subscription (EndByContextCancel). Refused once EndAll has run.
func (m *LocalSubscriptionManager) Subscribe(ctx context.Context, topics ...string) (*Subscription, error) {
	topics, err := distinctTopics(topics)
	if err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	ms := &managedSubscription{
		topics:     topics,
		queue:      make(chan *Message, m.conf.QueueCapacity),
		endNotice:  make(chan struct{}),
		missNotice: make(chan struct{}, 1),
	}
	sub := &Subscription{manager: m, managed: ms}
	ms.cleanup = runtime.AddCleanup(sub, func(ms *managedSubscription) { m.end(ms, EndByAbandonment) }, ms)

	m.allMu.Lock()
	if m.terminated {
		m.allMu.Unlock()
		ms.cleanup.Stop()
		return nil, errTerminated
	}
	m.all[ms] = struct{}{}
	m.allMu.Unlock()

	for _, t := range topics {
		sh := m.shardOf(t)
		sh.mu.Lock()
		// An end that ran meanwhile set ended before removing this subscription from the shards, so skipping here
		// leaves nothing behind.
		if !ms.ended.Load() {
			set := sh.topics[t]
			if set == nil {
				set = make(map[*managedSubscription]struct{})
				sh.topics[t] = set
			}
			set[ms] = struct{}{}
			m.noteCount(sh, t, len(set))
		}
		sh.mu.Unlock()
	}

	stop := context.AfterFunc(ctx, func() { m.end(ms, EndByContextCancel) })
	ms.stopContextWatch.Store(&stop)
	if ms.ended.Load() {
		stop() // ended before the stop function was stored: release the watch here
	}
	return sub, nil
}

// Deliver puts one Message into every subscription on topic. A subscription whose queue is full is ended
// (EndByFullQueue) instead of waited for.
func (m *LocalSubscriptionManager) Deliver(topic, payload string) {
	sh := m.shardOf(topic)
	var full []*managedSubscription
	sh.mu.Lock()
	if set := sh.topics[topic]; len(set) > 0 {
		msg := &Message{Topic: topic, Payload: payload}
		for ms := range set {
			if ms.ended.Load() {
				continue
			}
			select {
			case ms.queue <- msg:
			default:
				full = append(full, ms)
			}
		}
	}
	sh.mu.Unlock()
	for _, ms := range full {
		m.end(ms, EndByFullQueue)
	}
}

// PutMissNoticeToAll tells every subscription it manages that messages may have been missed. Each subscription's miss
// notice channel holds at most one notice; where an earlier one has not been taken yet, it already says this, and
// nothing is added.
func (m *LocalSubscriptionManager) PutMissNoticeToAll() {
	m.allMu.Lock()
	defer m.allMu.Unlock()
	for ms := range m.all {
		select {
		case ms.missNotice <- struct{}{}:
		default:
		}
	}
}

// EndAll ends every subscription it manages with EndByTermination, and refuses every Subscribe from then on.
func (m *LocalSubscriptionManager) EndAll() {
	m.allMu.Lock()
	m.terminated = true
	all := make([]*managedSubscription, 0, len(m.all))
	for ms := range m.all {
		all = append(all, ms)
	}
	m.allMu.Unlock()
	for _, ms := range all {
		m.end(ms, EndByTermination)
	}
}

// Stats returns a snapshot of the subscriptions it manages.
func (m *LocalSubscriptionManager) Stats() Stats {
	m.allMu.Lock()
	st := Stats{SubscriptionCount: len(m.all), SubscriptionCountPerTopic: make(map[string]int)}
	m.allMu.Unlock()
	for i := range m.shards {
		sh := &m.shards[i]
		sh.mu.Lock()
		for t, set := range sh.topics {
			st.SubscriptionCountPerTopic[t] = len(set)
		}
		sh.mu.Unlock()
	}
	return st
}

// TopicSubscriptionCountUpdatesNoticeReceiver returns the receive end of the channel that gets a notice whenever a
// topic's subscription count changes. At most one notice waits there; it means there are updates to take. nil when
// the updates are off: a nil channel is never ready.
func (m *LocalSubscriptionManager) TopicSubscriptionCountUpdatesNoticeReceiver() <-chan struct{} {
	return m.updatesNotice
}

// TakeTopicSubscriptionCountUpdates hands over every topic whose subscription count changed since the last take,
// with its latest count, and empties them. Empty when nothing changed or the updates are off.
func (m *LocalSubscriptionManager) TakeTopicSubscriptionCountUpdates() TopicSubscriptionCountUpdates {
	taken := make(TopicSubscriptionCountUpdates)
	for i := range m.shards {
		sh := &m.shards[i]
		sh.mu.Lock()
		if len(sh.updates) > 0 {
			for t, n := range sh.updates {
				taken[t] = n
			}
			sh.updates = make(TopicSubscriptionCountUpdates)
		}
		sh.mu.Unlock()
	}
	return taken
}

// noteCount records topic's new count for the owner when the updates are on, and puts a notice unless one waits.
// Called with sh.mu held: the update and its notice are written under the lock the count changed under, so a take
// that follows the notice sees the update.
func (m *LocalSubscriptionManager) noteCount(sh *shard, topic string, n int) {
	if sh.updates == nil {
		return
	}
	sh.updates[topic] = n
	select {
	case m.updatesNotice <- struct{}{}:
	default:
	}
}

// end ends ms once, whichever end comes first: the reason is stored before the end notice closes, so a reader woken by
// the close sees it; then ms leaves every shard and the manager's set, its waiting messages are dropped, and both
// watches are released.
func (m *LocalSubscriptionManager) end(ms *managedSubscription, reason EndReason) {
	ms.endOnce.Do(func() {
		ms.endReason.Store(int64(reason))
		ms.ended.Store(true)
		close(ms.endNotice)
		for _, t := range ms.topics {
			sh := m.shardOf(t)
			sh.mu.Lock()
			if set := sh.topics[t]; set != nil {
				if _, ok := set[ms]; ok {
					delete(set, ms)
					if len(set) == 0 {
						delete(sh.topics, t)
					}
					m.noteCount(sh, t, len(set))
				}
			}
			sh.mu.Unlock()
		}
		m.allMu.Lock()
		delete(m.all, ms)
		m.allMu.Unlock()
		// No put can reach ms any more (it left every shard), so this empties its queue for good.
	drain:
		for {
			select {
			case <-ms.queue:
			default:
				break drain
			}
		}
		if stop := ms.stopContextWatch.Load(); stop != nil {
			(*stop)()
		}
		ms.cleanup.Stop()
	})
}

// distinctTopics refuses an empty list or an empty topic and drops repeats, keeping the first-seen order. The result
// is a copy, so the caller's slice is free to change.
func distinctTopics(topics []string) ([]string, error) {
	if len(topics) == 0 {
		return nil, errors.New("pubsubs: subscribe names no topic")
	}
	out := make([]string, 0, len(topics))
	seen := make(map[string]struct{}, len(topics))
	for _, t := range topics {
		if t == "" {
			return nil, errors.New("pubsubs: subscribe names an empty topic")
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out, nil
}
