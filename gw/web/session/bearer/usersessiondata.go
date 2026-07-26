package bearer

import (
	"context"
	"sync"
)

// UserSessionData is the per-request bearer-session payload for user-bound
// sessions (sessions whose group includes "user" in its Binds). Attached to ctx
// by the auth middleware after resolving the access token; read by handlers.
//
// ClientID is "" for clientless user sessions.
type UserSessionData[UID comparable] struct {
	ID        string          // session ID (KVDB row key)
	UIDStr    string          // raw uid value from umbrella row
	UID       UID             // typed (parsed) value
	ClientID  string          // bound client ("" for clientless)
	GroupName string          // session group name
	Mgr       *SessionManager // back-ref to the process-scoped SessionManager

	upATknSlots sync.Map // clientID → *fwupstream.TknSlot (cached upstream access token, lazy)
	upRTknSlots sync.Map // clientID → *fwupstream.TknSlot (cached upstream refresh token, lazy)
}

func (sd *UserSessionData[UID]) SessionID() string { return sd.ID }

type ctxKeyBearerUserSession[UID comparable] struct{}

func WithUserSessionData[UID comparable](ctx context.Context, sd *UserSessionData[UID]) context.Context {
	return context.WithValue(ctx, ctxKeyBearerUserSession[UID]{}, sd)
}

func UserSessionDataFromContext[UID comparable](ctx context.Context) (*UserSessionData[UID], bool) {
	sd, ok := ctx.Value(ctxKeyBearerUserSession[UID]{}).(*UserSessionData[UID])
	return sd, ok
}
