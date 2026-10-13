package pubsubs

// EndReason says why a subscription ended. It is set before the subscription's end notice closes, so it can be read
// once the end notice receiver is ready; before that it is the zero value, which names no reason.
type EndReason int

const (
	_                  EndReason = iota
	EndByUnsubscribe             // the subscriber called Unsubscribe
	EndByContextCancel           // the context given to Subscribe was canceled
	EndByFullQueue               // a message came while the message queue was full: the subscriber was not taking
	EndByTermination             // the broker was terminated
	EndByAbandonment             // no code held the Subscription any more, so none could take from it
)

func (r EndReason) String() string {
	switch r {
	case EndByUnsubscribe:
		return "unsubscribe"
	case EndByContextCancel:
		return "context cancel"
	case EndByFullQueue:
		return "full queue"
	case EndByTermination:
		return "termination"
	case EndByAbandonment:
		return "abandonment"
	default:
		return "not ended"
	}
}
