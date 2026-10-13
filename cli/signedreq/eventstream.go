package main

import (
	"bufio"
	"bytes"
	"encoding/json/v2"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
)

// eventStream is what a reader of an event stream keeps across connections, as a browser's EventSource does: the last
// event ID, sent as Last-Event-ID when the stream is reopened, and how long to wait before reopening, which the
// stream's retry: field sets.
type eventStream struct {
	lastEventID string
	reconnect   time.Duration
}

// read parses one connection's body as text/event-stream (WHATWG HTML, "Server-sent events") and prints each event it
// dispatches to out as one JSON line: {"id": the last event ID, "event": its type, "data": its data}. It returns when
// the body ends — nil at its end, else the read error; an event not finished by then is dropped.
func (s *eventStream) read(body io.Reader, out io.Writer) error {
	r := bufio.NewReader(body)
	var (
		line, data         []byte
		eventType          string
		idBuffer           = s.lastEventID
		afterCR, firstLine = false, true
	)
	dispatch := func() {
		s.lastEventID = idBuffer
		if len(data) == 0 {
			eventType = ""
			return
		}
		if eventType == "" {
			eventType = "message"
		}
		b, err := json.Marshal(struct {
			ID    string `json:"id"`
			Event string `json:"event"`
			Data  string `json:"data"`
		}{
			strings.ToValidUTF8(s.lastEventID, "\uFFFD"),
			strings.ToValidUTF8(eventType, "\uFFFD"),
			strings.ToValidUTF8(string(bytes.TrimSuffix(data, []byte("\n"))), "\uFFFD"),
		})
		if err == nil {
			_, _ = out.Write(append(b, '\n'))
		}
		data, eventType = data[:0], ""
	}
	for {
		c, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		if afterCR { // a line ends at CRLF, LF or CR
			afterCR = false
			if c == '\n' {
				continue
			}
		}
		if c != '\n' && c != '\r' {
			line = append(line, c)
			continue
		}
		afterCR = c == '\r'
		if firstLine {
			line = bytes.TrimPrefix(line, []byte("\uFEFF"))
			firstLine = false
		}
		field, value, hasColon := bytes.Cut(line, []byte(":"))
		switch {
		case len(line) == 0:
			dispatch()
		case len(field) == 0: // a comment
		default:
			if hasColon {
				value = bytes.TrimPrefix(value, []byte(" "))
			}
			switch string(field) {
			case "event":
				eventType = string(value)
			case "data":
				data = append(append(data, value...), '\n')
			case "id":
				if bytes.IndexByte(value, 0) < 0 {
					idBuffer = string(value)
				}
			case "retry": // only ASCII digits count; anything else is ignored
				if ms, err := strconv.ParseUint(string(value), 10, 64); err == nil && ms <= uint64(math.MaxInt64/time.Millisecond) {
					s.reconnect = time.Duration(ms) * time.Millisecond
				}
			}
		}
		line = line[:0]
	}
}
