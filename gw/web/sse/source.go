package sse

import "errors"

// Source is what one stream sends: items of any type from one producer, and how each becomes a frame.
type Source[T any] struct {
	Items     <-chan T        // REQUIRED. What to send, in order. Closing it ends the stream.
	ToEvent   func(T) Event   // REQUIRED. The app's translation of one item; an Event it cannot frame ends the stream with an error.
	EndNotice <-chan struct{} // optional: closed when the producer has ended; nil = only Items closing ends the stream
	Backlog   []Event         // written first: what the client missed before this stream (see LastEventID)
	Heartbeat Heartbeat       // REQUIRED: what the stream sends when it has been quiet
}

func (s Source[T]) validate() error {
	if s.Items == nil {
		return errors.New("sse: source has no items channel")
	}
	if s.ToEvent == nil {
		return errors.New("sse: source has no ToEvent")
	}
	for _, ev := range s.Backlog {
		if err := ev.validate(); err != nil {
			return err
		}
	}
	return s.Heartbeat.validate()
}
