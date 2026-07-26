package framework

import (
	"encoding/base64"
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
// user-bound and anonymous shapes. Reads .cookie-session.json with the
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

	confFilePath := filepath.Join(c.AppRoot, "config", ".cookie-session.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return err
	}

	var raw map[string]jsontext.Value
	if err := json.Unmarshal(confBytes, &raw); err != nil {
		return err
	}

	sessionConf := &cookie.SessionConf{}
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

	// enckey is stored base64-encoded in config (32 random bytes → base64).
	// Generate with: openssl rand -base64 32
	if sessionConf.UserSession != nil {
		if err := sessionConf.UserSession.Validate(); err != nil {
			return err
		}
		keyBytes, err := base64.StdEncoding.DecodeString(sessionConf.UserSession.EncryptionKey)
		if err != nil {
			return fmt.Errorf("user cookie enckey is not valid base64: %v", err)
		}
		cipher, err := security.NewXChaCha20Poly1305CipherBase64(keyBytes) // validates len == 32 post-decode
		if err != nil {
			return fmt.Errorf("user cookie cipher: %v", err)
		}
		mgr.UserCookieCipher = cipher
	}
	if sessionConf.AnonymousSession != nil {
		if err := sessionConf.AnonymousSession.Validate(); err != nil {
			return err
		}
		keyBytes, err := base64.StdEncoding.DecodeString(sessionConf.AnonymousSession.EncryptionKey)
		if err != nil {
			return fmt.Errorf("anonymous cookie enckey is not valid base64: %v", err)
		}
		cipher, err := security.NewXChaCha20Poly1305CipherBase64(keyBytes) // validates len == 32 post-decode
		if err != nil {
			return fmt.Errorf("anonymous cookie cipher: %v", err)
		}
		mgr.AnonymousCookieCipher = cipher
	}

	c.SessionService.CookieSessionManager = mgr
	mgr.Enable() // cookie protocol wired → serving
	return nil
}
