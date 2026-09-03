package cookie

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

func (m *SessionManager) AnonymousSessionRowKey(sessionID string) string {
	return m.appName + ":ca:" + sessionID
}

func (m *SessionManager) AnonymousSessionRowExists(ctx context.Context, sessionID string) (bool, error) {
	return m.KVDB.Exists(ctx, m.AnonymousSessionRowKey(sessionID))
}

// CreateAnonymousSession creates a complete anonymous session: stores the row in
// KVDB via StoreAnonymousSession, then sets the browser cookie via
// SetAnonymousSessionCookie.
func (m *SessionManager) CreateAnonymousSession(ctx context.Context, w http.ResponseWriter) (string, error) {
	sid, err := m.StoreAnonymousSession(ctx)
	if err != nil {
		return "", err
	}
	if err := m.SetAnonymousSessionCookie(w, sid); err != nil {
		return "", err
	}
	return sid, nil
}

// StoreAnonymousSession creates a new anonymous session: writes the umbrella row
// to KVDB (csrf only, with sliding TTL). Returns the generated sessionID.
// Anonymous sessions have no per-user cap. The caller is responsible for setting
// the cookie via SetAnonymousSessionCookie.
func (m *SessionManager) StoreAnonymousSession(ctx context.Context) (string, error) {
	sessionID := security.GenerateHex(16)
	csrfTkn := security.GenerateBase64RawURL(32)
	slidingExpiration := time.Duration(m.Conf.AnonymousSession.ExpireIn) * time.Second
	key := m.AnonymousSessionRowKey(sessionID)
	fields := map[string]any{"csrf": csrfTkn}
	if err := m.KVDB.HashSetFieldsWithKeyTTL(ctx, key, fields, slidingExpiration); err != nil {
		return "", err
	}
	return sessionID, nil
}

// FetchAnonymousSession reads the anonymous session row from KVDB.
// Returns (nil, nil) if the row doesn't exist.
func (m *SessionManager) FetchAnonymousSession(ctx context.Context, sessionID string) (*AnonymousSessionRow, error) {
	fields, err := m.KVDB.HashGetFields(ctx, m.AnonymousSessionRowKey(sessionID), "csrf")
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return &AnonymousSessionRow{
		CSRF: fields["csrf"],
	}, nil
}

// DestroyAnonymousSession destroys an anonymous session completely: deletes KVDB
// rows via DeleteAnonymousSessionKVDB and removes the browser cookie via
// DeleteAnonymousSessionCookie. Cookie removal always runs even if KVDB errored.
func (m *SessionManager) DestroyAnonymousSession(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	err := m.DeleteAnonymousSessionKVDB(ctx, sessionID)
	m.DeleteAnonymousSessionCookie(w)
	return err
}

// DeleteAnonymousSessionKVDB removes an anonymous session's umbrella row. Idempotent.
// Anonymous sessions have no cap list, so this is a single row delete.
func (m *SessionManager) DeleteAnonymousSessionKVDB(ctx context.Context, sessionID string) error {
	_, _ = m.KVDB.Delete(ctx, m.AnonymousSessionRowKey(sessionID))
	return nil
}

// ExtendAnonymousSession extends both the KVDB-side TTL and the browser cookie's
// MaxAge for an anonymous session. Best-effort.
func (m *SessionManager) ExtendAnonymousSession(ctx context.Context, w http.ResponseWriter, sessionID, encCookieValue string) {
	m.ExtendAnonymousSessionKVDB(ctx, sessionID)
	m.ExtendAnonymousSessionCookie(w, encCookieValue)
}

// ExtendAnonymousSessionKVDB extends the sliding TTL on the anonymous session
// umbrella row using Conf.AnonymousSession.ExpireIn. Best-effort.
func (m *SessionManager) ExtendAnonymousSessionKVDB(ctx context.Context, sessionID string) {
	m.ExtendAnonymousSessionKVDBWithTTL(ctx, sessionID,
		time.Duration(m.Conf.AnonymousSession.ExpireIn)*time.Second)
}

// ExtendAnonymousSessionKVDBWithTTL extends the TTL on the anonymous session
// umbrella row by the given ttl. Best-effort — Expire silently no-ops if the
// row doesn't exist.
func (m *SessionManager) ExtendAnonymousSessionKVDBWithTTL(ctx context.Context, sessionID string, ttl time.Duration) {
	_, _ = m.KVDB.Expire(ctx, m.AnonymousSessionRowKey(sessionID), ttl)
}

// SetAnonymousSessionCookie writes the Set-Cookie HTTP response header for an
// anonymous session, with the sessionID encrypted via AnonymousCookieCipher.
// MaxAge matches Conf.AnonymousSession.ExpireIn. HttpOnly + Secure + SameSite=Lax.
func (m *SessionManager) SetAnonymousSessionCookie(w http.ResponseWriter, sessionID string) error {
	encSessionID, err := m.AnonymousCookieCipher.EncryptEncode([]byte(sessionID), m.AnonymousCookieCipherContext())
	if err != nil {
		return fmt.Errorf("failed to encrypt anonymous session id. %v", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     AnonymousCookieName,
		Value:    encSessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   m.Conf.AnonymousSession.ExpireIn,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ExtendAnonymousSessionCookie resets the anonymous session cookie's MaxAge using
// Conf.AnonymousSession.ExpireIn. Same encrypted value, new lifetime.
func (m *SessionManager) ExtendAnonymousSessionCookie(w http.ResponseWriter, encValue string) {
	m.ExtendAnonymousSessionCookieWithMaxAge(w, encValue, m.Conf.AnonymousSession.ExpireIn)
}

// ExtendAnonymousSessionCookieWithMaxAge resets the anonymous session cookie's
// MaxAge to the given seconds, keeping the same encrypted value.
func (m *SessionManager) ExtendAnonymousSessionCookieWithMaxAge(w http.ResponseWriter, encValue string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     AnonymousCookieName,
		Value:    encValue, // SAME encrypted value
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   maxAge,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) DeleteAnonymousSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     AnonymousCookieName,
		Path:     "/",
		MaxAge:   -1, // Delete
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) VerifyAnonymousSessionCookie(ctx context.Context, r *http.Request) *errs.Error {
	sessionCookie, err := r.Cookie(AnonymousCookieName)
	if err != nil {
		return errs.CookieNotFound
	}
	cookieSessionID, err := m.AnonymousCookieCipher.DecodeDecrypt(sessionCookie.Value, m.AnonymousCookieCipherContext())
	if err != nil {
		return errs.InvalidCookie.WithCause(err)
	}
	found, err := m.AnonymousSessionRowExists(ctx, string(cookieSessionID))
	if err != nil {
		return errs.KVDB.WithDetail("verify anonymous session cookie").WithCause(err)
	}
	if !found {
		return errs.CookieSessionNotFound
	}
	return nil
}
