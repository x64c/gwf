package cookie

import (
	"context"
	"sync"
)

// AnonymousSessionData is the per-request cookie-session payload for sessions
// not bound to any user. Attached to ctx by the auth middleware and read by handlers.
type AnonymousSessionData struct {
	ID      string          // session ID (KVDB row key)
	CSRFTkn string          // CSRF token bound to this session
	Mgr     *SessionManager // back-ref to the process-scoped SessionManager

	upATknSlots sync.Map // clientID → *fwupstream.TknSlot (cached upstream access token, lazy)
	upRTknSlots sync.Map // clientID → *fwupstream.TknSlot (cached upstream refresh token, lazy)
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
