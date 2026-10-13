package pubsubs

// Message is one published message as a subscription receives it: the topic it was published to and its payload,
// both exactly as the publisher gave them. A publish makes one Message and every subscription's queue holds a pointer
// to that same value, so a subscriber must not change it.
type Message struct {
	Topic   string
	Payload string
}
