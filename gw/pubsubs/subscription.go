package pubsubs

// Subscription is the subscriber's hold on a subscription. Take messages from MessageQueueReceiver. EndNoticeReceiver
// closes once, when the subscription ends, and EndReason then says why. MissNoticeReceiver gets a notice when messages
// may have been missed while the subscription went on.
//
// Keep the Subscription itself — not only its channels — for as long as you take from it: once no code holds it, the
// subscription is ended (EndByAbandonment).
type Subscription struct {
	manager *LocalSubscriptionManager
	managed *managedSubscription
}

func (s *Subscription) MessageQueueReceiver() <-chan *Message {
	return s.managed.queue
}

func (s *Subscription) EndNoticeReceiver() <-chan struct{} {
	return s.managed.endNotice
}

func (s *Subscription) MissNoticeReceiver() <-chan struct{} {
	return s.managed.missNotice
}

// EndReason says why the subscription ended; the zero value until the end notice closes.
func (s *Subscription) EndReason() EndReason {
	return EndReason(s.managed.endReason.Load())
}

// Unsubscribe ends the subscription now (EndByUnsubscribe). It does nothing once the subscription has ended.
func (s *Subscription) Unsubscribe() {
	s.manager.end(s.managed, EndByUnsubscribe)
}
