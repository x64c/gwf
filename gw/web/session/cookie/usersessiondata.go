package cookie

import (
	"context"

	"github.com/x64c/gwf/gw/web/fwupstream"
)

// UserSessionData is the per-request cookie-session payload for sessions
// bound to a user. Attached to ctx by the auth middleware and read by handlers.
type UserSessionData[UID comparable] struct {
	ID      string          // session ID (KVDB row key)
	UIDStr  string          // raw value from KVDB
	UID     UID             // typed (parsed) value
	CSRFTkn string          // CSRF token bound to this session
	Mgr     *SessionManager // back-ref to the process-scoped SessionManager

	upATknSlots fwupstream.TknSlots // cached upstream access-token reads, lazy, keyed by client id
	upRTknSlots fwupstream.TknSlots // cached upstream refresh-token reads, lazy, keyed by client id
}

func (sd *UserSessionData[UID]) SessionID() string { return sd.ID }

type ctxKeyUserSession[UID comparable] struct{}

func WithUserSessionData[UID comparable](ctx context.Context, sd *UserSessionData[UID]) context.Context {
	return context.WithValue(ctx, ctxKeyUserSession[UID]{}, sd)
}

func UserSessionDataFromContext[UID comparable](ctx context.Context) (*UserSessionData[UID], bool) {
	sd, ok := ctx.Value(ctxKeyUserSession[UID]{}).(*UserSessionData[UID])
	return sd, ok
}
