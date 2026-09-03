package cookie

import (
	"errors"
	"sync/atomic"

	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/locking"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

// SessionManager owns cookie-session operations across both user and anonymous shapes.
// Each shape's state (Conf sub-block, Cipher) is present iff that shape is configured.
type SessionManager struct {
	Conf *SessionConf

	UserCookieCipher      security.EncodedCipher // present iff Conf.UserSession != nil
	AnonymousCookieCipher security.EncodedCipher // present iff Conf.AnonymousSession != nil

	FWUpstream *fwupstream.Hub // upstream subsystem; token I/O delegates here. nil iff no upstream configured

	appName        string
	KVDB           kvdbs.DB        // holds session rows
	lockingManager locking.Manager // set at construction; shared with session.Service; guards each session's upstream refresh

	enabled atomic.Bool // the cookie protocol's on/off switch (svc.Switchable)
}

// NewSessionManager builds a SessionManager for the app named appName, over
// kvdb, with lockingManager guarding each session's upstream refresh. The
// app's name and the locking manager are sealed here: every row key and every
// cookie cipher context derives from the name, and every instance of the app
// must hold its refresh names on the same manager, so neither may change
// after construction. The shape-specific fields — Conf, the two ciphers,
// FWUpstream — are the caller's to set. None of the three arguments may be
// empty.
func NewSessionManager(appName string, kvdb kvdbs.DB, lockingManager locking.Manager) (*SessionManager, error) {
	if appName == "" {
		return nil, errors.New("cookie.NewSessionManager: appName required")
	}
	if kvdb == nil {
		return nil, errors.New("cookie.NewSessionManager: kvdb required")
	}
	if lockingManager == nil {
		return nil, errors.New("cookie.NewSessionManager: lockingManager required")
	}
	return &SessionManager{appName: appName, KVDB: kvdb, lockingManager: lockingManager}, nil
}

// UserCookieCipherContext / AnonymousCookieCipherContext are the cipher
// contexts cookie values are bound to: cookie name + app. Cookie names are
// package constants shared by every gwf app, so App is what keeps two apps'
// cookies from decrypting each other's. Every seal/open of a cookie value
// goes through these — never a hand-built context.
func (m *SessionManager) UserCookieCipherContext() security.CipherContext {
	return security.CipherContext{App: m.appName, Location: UserCookieName}
}

func (m *SessionManager) AnonymousCookieCipherContext() security.CipherContext {
	return security.CipherContext{App: m.appName, Location: AnonymousCookieName}
}

// Enable / Disable / Enabled implement svc.Switchable — the cookie protocol's
// own on/off switch. Enabled() reports only this switch; whether the SERVICE
// may be used is not the manager's to answer — the caller's framework handle
// already did (svc.Service: methods judge no availability). The manager keeps
// no lifecycle state and no back-pointer to its service on purpose: a Serving()
// self-verdict here was a second authority beside admission, and the two
// diverge under abandonment.
func (m *SessionManager) Enable()       { m.enabled.Store(true) }
func (m *SessionManager) Disable()      { m.enabled.Store(false) }
func (m *SessionManager) Enabled() bool { return m.enabled.Load() }
