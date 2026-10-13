package sse

import (
	"errors"
	"strings"
)

// Event is one SSE frame. ID becomes the client's Last-Event-ID; Name selects the client-side listener (empty means
// the default "message"); Data may span lines.
type Event struct {
	ID   string
	Name string
	Data string
}

// validate refuses what would break the frame: a line break in ID or Name ends the field early and the rest would be
// read as another field; a NUL in ID makes a browser ignore the id.
func (e Event) validate() error {
	if strings.ContainsAny(e.ID, "\r\n\x00") {
		return errors.New("sse: event id contains a line break or NUL")
	}
	if strings.ContainsAny(e.Name, "\r\n") {
		return errors.New("sse: event name contains a line break")
	}
	return nil
}

// appendFrame appends the event's wire form to b: id, event, one data line per line of Data — at least one, because
// a client never dispatches an event without a data line — and the blank line that ends the event.
func (e Event) appendFrame(b []byte) []byte {
	if e.ID != "" {
		b = append(b, "id: "...)
		b = append(b, e.ID...)
		b = append(b, '\n')
	}
	if e.Name != "" {
		b = append(b, "event: "...)
		b = append(b, e.Name...)
		b = append(b, '\n')
	}
	data := e.Data
	for {
		i := strings.IndexAny(data, "\r\n")
		b = append(b, "data: "...)
		if i < 0 {
			b = append(b, data...)
			b = append(b, '\n')
			break
		}
		b = append(b, data[:i]...)
		b = append(b, '\n')
		if data[i] == '\r' && i+1 < len(data) && data[i+1] == '\n' {
			i++
		}
		data = data[i+1:]
	}
	return append(b, '\n')
}
