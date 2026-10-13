package pubsubs

// TopicSubscriptionCountUpdates maps each topic whose subscription count changed since the last take to its latest
// count. A topic that changed several times appears once, with the count it has now; 0 means none is left.
type TopicSubscriptionCountUpdates map[string]int
