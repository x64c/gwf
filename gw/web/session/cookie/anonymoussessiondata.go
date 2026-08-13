package cookie

import (
	"context"

	"github.com/x64c/gwf/gw/web/fwupstream"
)

// AnonymousSessionData is the per-request cookie-session payload for sessions
// not bound to any user. Attached to ctx by the auth middleware and read by handlers.
type AnonymousSessionData struct {
	ID      string          // session ID (KVDB row key)
	CSRFTkn string          // CSRF token bound to this session
	Mgr     *SessionManager // back-ref to the process-scoped SessionManager

	upATknSlots fwupstream.TknSlots // cached upstream access-token reads, lazy, keyed by client id
	upRTknSlots fwupstream.TknSlots // cached upstream refresh-token reads, lazy, keyed by client id
}

func (sd *AnonymousSessionData) SessionID() string { return sd.ID }

type ctxKeyAnonymousSession struct{}

func WithAnonymousSessionData(ctx context.Context, sd *AnonymousSessionData) context.Context {
	return context.WithValue(ctx, ctxKeyAnonymousSession{}, sd)
}

func AnonymousSessionDataFromContext(ctx context.Context) (*AnonymousSessionData, bool) {
	sd, ok := ctx.Value(ctxKeyAnonymousSession{}).(*AnonymousSessionData)
	return sd, ok
}
