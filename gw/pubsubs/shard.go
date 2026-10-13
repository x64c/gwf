package pubsubs

import "sync"

// shard is one of a manager's lock stripes: the subscriptions of the topics that hash to it, and those topics' count
// updates not yet taken.
type shard struct {
	mu      sync.Mutex
	topics  map[string]map[*managedSubscription]struct{}
	updates TopicSubscriptionCountUpdates // nil when the updates are off
}
