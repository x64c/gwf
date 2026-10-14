package sse

import (
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/web"
)

// Serve runs one stream on this request's response until it ends. Call it from a route handler once the handler has
// decided the caller may have the stream; the handler writes nothing to the response before it or after it.
//
// Before anything is written, Serve answers 500 for a Source that is incomplete or carries an event that would break
// the frame, or for a ResponseWriter that hides write deadlines (a wrapper's own writer without Unwrap(); every frame
// sets one), and 204 when the service is not running (RefuseStream), so a client does not come back. Then it sends the
// stream's headers and the Backlog, and stays: a frame for each item, and the heartbeat after each Heartbeat.Silence
// without a write. It ends when Items or EndNotice closes, when the web service starts its drain, when this service
// stops, when the client leaves, or when a write fails or does not finish within frame_write_timeout_secs. On the
// first three the client is told when to reconnect (retry:). Returns nil for every ending but a refusal, a failed
// write, or an item ToEvent turns into an event that cannot be framed.
func Serve[T any](s *Service, w http.ResponseWriter, r *http.Request, src Source[T]) error {
	if err := src.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	// Every frame sets a write deadline and flushes through rc. A writer that hides them would end the stream at its
	// first write, after a 200 that a client takes for a stream to come back to.
	rc := http.NewResponseController(w)
	if err := rc.SetWriteDeadline(time.Now().Add(s.conf.frameWriteTimeout())); err != nil {
		err = fmt.Errorf("sse: the ResponseWriter hides write deadlines and flushing (a wrapper in front passes on a writer of its own without Unwrap() http.ResponseWriter): %w", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return err
	}
	cycleDone, ok := s.register()
	if !ok {
		RefuseStream(w, http.StatusNoContent)
		return errNotRunning
	}
	defer s.unregister()

	setStreamHeaders(w)
	w.WriteHeader(http.StatusOK)
	buf := make([]byte, 0, 512)
	for _, ev := range src.Backlog {
		buf = ev.appendFrame(buf)
	}
	if err := s.write(w, rc, buf); err != nil {
		s.noteWriteError(err)
		return err
	}

	heartbeat := src.Heartbeat.frame()
	silence := time.NewTimer(src.Heartbeat.Silence)
	defer silence.Stop()
	drainNotice, _ := web.DrainNoticeReceiverFromContext(r.Context())
	for {
		select {
		case item, more := <-src.Items:
			if !more {
				s.endWithRetry(w, rc)
				return nil
			}
			ev := src.ToEvent(item)
			if err := ev.validate(); err != nil {
				return err
			}
			if err := s.write(w, rc, ev.appendFrame(buf[:0])); err != nil {
				s.noteWriteError(err)
				return err
			}
			silence.Reset(src.Heartbeat.Silence)
		case <-silence.C:
			if err := s.write(w, rc, heartbeat); err != nil {
				s.noteWriteError(err)
				return err
			}
			silence.Reset(src.Heartbeat.Silence)
		case <-src.EndNotice:
			s.endWithRetry(w, rc)
			return nil
		case <-drainNotice:
			s.endWithRetry(w, rc)
			return nil
		case <-cycleDone:
			s.endWithRetry(w, rc)
			return nil
		case <-r.Context().Done():
			return nil
		}
	}
}
