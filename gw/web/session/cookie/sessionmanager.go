package cookie

import (
	"sync/atomic"

	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/fwupstream"
	"github.com/x64c/gwf/gw/web/session/lockstore"
)

// SessionManager owns cookie-session operations across both user and anonymous shapes.
// Each shape's state (Conf sub-block, Cipher) is present iff that shape is configured.
type SessionManager struct {
	Conf *SessionConf

	UserCookieCipher      security.EncodedCipher // present iff Conf.UserSession != nil
	AnonymousCookieCipher security.EncodedCipher // present iff Conf.AnonymousSession != nil

	FWUpstream *fwupstream.Hub // upstream subsystem; token I/O delegates here. nil iff this app has no upstream

	AppName      string
	KVDB         kvdbs.DB // holds session rows
	SessionLocks *lockstore.Store

	enabled atomic.Bool // the cookie protocol's on/off switch (svc.Switchable)
}

// UserCookieCipherContext / AnonymousCookieCipherContext are the cipher
// contexts cookie values are bound to: cookie name + app. Cookie names are
// package constants shared by every gwf app, so App is what keeps two apps'
// cookies from decrypting each other's. Every seal/open of a cookie value
// goes through these — never a hand-built context.
func (m *SessionManager) UserCookieCipherContext() security.CipherContext {
	return security.CipherContext{App: m.AppName, Location: UserCookieName}
}

func (m *SessionManager) AnonymousCookieCipherContext() security.CipherContext {
	return security.CipherContext{App: m.AppName, Location: AnonymousCookieName}
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
