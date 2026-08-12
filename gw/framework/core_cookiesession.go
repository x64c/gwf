package framework

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// PrepareCookieSessions prepares the cookie session manager covering both
// user-bound and anonymous shapes. Reads .web-cookie-session.json with the
// "user" key (user shape) and "" key (anonymous shape) — either or both.
// The manager is attached to SessionService.CookieSessionManager.
// Prerequisite: MainKVDB, SessionService.
//
// Pass useFWUpstream=true if cookie sessions store upstream tokens; the manager
// then takes c.FWUpstream, and this fails at boot if it isn't prepared yet
// (call PrepareFWUpstream first). Pass false if cookie sessions have no upstream.
func (c *Core) PrepareCookieSessions(useFWUpstream bool) error {
	if c.SessionService == nil {
		return errors.New("session service not ready")
	}

	confFilePath := filepath.Join(c.AppRoot, "config", ".web-cookie-session.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return err
	}

	var raw map[string]jsontext.Value
	if err := json.Unmarshal(confBytes, &raw); err != nil {
		return err
	}

	sessionConf := &cookie.SessionConf{}
	if keyringBytes, ok := raw["keyring"]; ok {
		sessionConf.Keyring = &security.KeyringConf{}
		if err := json.Unmarshal(keyringBytes, sessionConf.Keyring); err != nil {
			return fmt.Errorf("unmarshal keyring: %v", err)
		}
	}
	if userBytes, ok := raw["user"]; ok {
		sessionConf.UserSession = &cookie.UserSessionConf{}
		if err := json.Unmarshal(userBytes, sessionConf.UserSession); err != nil {
			return fmt.Errorf("unmarshal user shape: %v", err)
		}
	}
	if anonBytes, ok := raw[""]; ok {
		sessionConf.AnonymousSession = &cookie.AnonymousSessionConf{}
		if err := json.Unmarshal(anonBytes, sessionConf.AnonymousSession); err != nil {
			return fmt.Errorf("unmarshal anonymous shape: %v", err)
		}
	}

	mgr := &cookie.SessionManager{
		Conf:          sessionConf,
		AppName:       c.AppName,
		KVDB:          c.MainKVDB,
		SessionLocks:  c.SessionService.SessionLocks,
		ParentService: c.SessionService,
	}

	// Wire the upstream subsystem only when the app declares cookie sessions
	// store upstream tokens — and verify it was prepared, failing loud at boot.
	if useFWUpstream {
		if c.FWUpstream == nil {
			return errors.New("cookie sessions: useFWUpstream=true but FWUpstream not prepared — call PrepareFWUpstream first")
		}
		mgr.FWUpstream = c.FWUpstream
	}

	// One keyring serves both shapes; each shape's cipher is derived under its
	// own purpose label. Construction validates the whole keyring (active
	// among keys, key ids, algs, key material) — misconfiguration is a boot
	// failure here, never a per-request surprise. Each key's "enckey" is
	// base64-encoded 32 random master bytes: openssl rand -base64 32
	if sessionConf.UserSession != nil || sessionConf.AnonymousSession != nil {
		if sessionConf.Keyring == nil {
			return errors.New("cookie sessions: no \"keyring\" in .web-cookie-session.json")
		}
	}
	if sessionConf.UserSession != nil {
		if err := sessionConf.UserSession.Validate(); err != nil {
			return err
		}
		cipher, err := security.NewKeyringCipher(sessionConf.Keyring, cookie.UserCookieCipherPurpose)
		if err != nil {
			return fmt.Errorf("user cookie cipher: %v", err)
		}
		mgr.UserCookieCipher = cipher
	}
	if sessionConf.AnonymousSession != nil {
		if err := sessionConf.AnonymousSession.Validate(); err != nil {
			return err
		}
		cipher, err := security.NewKeyringCipher(sessionConf.Keyring, cookie.AnonymousCookieCipherPurpose)
		if err != nil {
			return fmt.Errorf("anonymous cookie cipher: %v", err)
		}
		mgr.AnonymousCookieCipher = cipher
	}

	c.SessionService.CookieSessionManager = mgr
	mgr.Enable() // cookie protocol wired → serving
	return nil
}
