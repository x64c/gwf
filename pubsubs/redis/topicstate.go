package redis

// topicState is where a topic stands on the broker's subscriber connection. The zero value is "not on the connection":
// a topic not in the broker's map.
type topicState int

const (
	_                topicState = iota
	topicSubscribing            // SUBSCRIBE sent, its confirmation not yet in
	topicSubscribed             // confirmed: Redis sends this connection the topic's messages
)
