package pubsubs

// LocalSubscriptionManagerOptions are the optional parts of a LocalSubscriptionManager.
type LocalSubscriptionManagerOptions struct {
	// TopicSubscriptionCountUpdates keeps each topic's latest subscription count as it changes, for the owner to take
	// (TakeTopicSubscriptionCountUpdates) when notified (TopicSubscriptionCountUpdatesNoticeReceiver). Left false,
	// nothing is kept and no notice is put. Turn it on only when something takes them: kept updates grow by one entry
	// per changed topic until taken.
	TopicSubscriptionCountUpdates bool
}
