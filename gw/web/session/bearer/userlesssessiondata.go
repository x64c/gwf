package bearer

import (
	"context"

	"github.com/x64c/gwf/gw/web/fwupstream"
)

// UserlessSessionData is the per-request bearer-session payload for sessions
// without a bound user (sessions whose group does not include "user" in its
// Binds — e.g. machine-to-machine client tokens, anonymous public API tokens).
//
// ClientID is "" for fully anonymous sessions.
type UserlessSessionData struct {
	ID        string          // session ID (KVDB row key)
	ClientID  string          // bound client ("" for fully anonymous)
	GroupName string          // session group name
	Mgr       *SessionManager // back-ref to the process-scoped SessionManager

	upATknSlots fwupstream.TknSlots // cached upstream access-token reads, lazy, keyed by client id
	upRTknSlots fwupstream.TknSlots // cached upstream refresh-token reads, lazy, keyed by client id
}

func (sd *UserlessSessionData) SessionID() string { return sd.ID }

type ctxKeyBearerUserlessSession struct{}

func WithUserlessSessionData(ctx context.Context, sd *UserlessSessionData) context.Context {
	return context.WithValue(ctx, ctxKeyBearerUserlessSession{}, sd)
}

func UserlessSessionDataFromContext(ctx context.Context) (*UserlessSessionData, bool) {
	sd, ok := ctx.Value(ctxKeyBearerUserlessSession{}).(*UserlessSessionData)
	return sd, ok
}
