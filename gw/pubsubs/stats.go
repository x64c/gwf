package pubsubs

// Stats is a snapshot of a broker's subscriptions.
type Stats struct {
	SubscriptionCount         int            // open now
	SubscriptionCountPerTopic map[string]int // open subscriptions per topic; a subscription to several topics counts under each
}
