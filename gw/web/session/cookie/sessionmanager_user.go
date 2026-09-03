package cookie

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
	"github.com/x64c/gwf/gw/web/session/caplist"
)

func (m *SessionManager) UserSessionRowKey(sessionID string) string {
	return m.appName + ":cu:" + sessionID
}

func (m *SessionManager) UserSessionRowExists(ctx context.Context, sessionID string) (bool, error) {
	return m.KVDB.Exists(ctx, m.UserSessionRowKey(sessionID))
}

// CreateUserSession creates a complete user session: stores the row in KVDB
// via StoreUserSession, then sets the browser cookie via SetUserSessionCookie.
func (m *SessionManager) CreateUserSession(ctx context.Context, w http.ResponseWriter, uidStr string) (string, error) {
	sid, err := m.StoreUserSession(ctx, uidStr)
	if err != nil {
		return "", err
	}
	if err := m.SetUserSessionCookie(w, sid); err != nil {
		return "", err
	}
	return sid, nil
}

// StoreUserSession creates a new user session: writes the umbrella row to KVDB
// (uid + csrf, with sliding TTL), and pushes the new sid into the per-user cap
// list (if MaxSessionsPerUser > 0), evicting oldest sessions if over the cap.
// Returns the generated sessionID. The caller is responsible for setting the
// cookie via SetUserSessionCookie.
func (m *SessionManager) StoreUserSession(ctx context.Context, uidStr string) (string, error) {
	sessionID := security.GenerateHex(16)
	csrfTkn := security.GenerateBase64RawURL(32)
	slidingExpiration := time.Duration(m.Conf.UserSession.ExpireIn) * time.Second
	key := m.UserSessionRowKey(sessionID)
	fields := map[string]any{"uid": uidStr, "csrf": csrfTkn}
	if err := m.KVDB.HashSetFieldsWithKeyTTL(ctx, key, fields, slidingExpiration); err != nil {
		return "", err
	}

	if m.Conf.UserSession.MaxSessionsPerUser > 0 {
		usrSessionListKey := fmt.Sprintf("%s:cul:%s", m.appName, uidStr)
		// UserSessionRowKey("") is the row-key prefix: each evicted session's row is deleted at prefix + sid.
		if err := caplist.PushEvictOverCap(ctx, m.KVDB, usrSessionListKey, sessionID, m.Conf.UserSession.MaxSessionsPerUser, slidingExpiration, m.UserSessionRowKey("")); err != nil {
			return "", err
		}
	}

	return sessionID, nil
}

// FetchUserSession reads the user session row from KVDB.
// Returns (nil, nil) if the row doesn't exist (session expired or never existed).
func (m *SessionManager) FetchUserSession(ctx context.Context, sessionID string) (*UserSessionRow, error) {
	fields, err := m.KVDB.HashGetFields(ctx, m.UserSessionRowKey(sessionID), "uid", "csrf")
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, nil
	}
	return &UserSessionRow{
		UID:  fields["uid"],
		CSRF: fields["csrf"],
	}, nil
}

// DestroyUserSession destroys a user session completely: deletes KVDB rows via
// DeleteUserSessionKVDB and removes the browser cookie via DeleteUserSessionCookie.
// The cookie removal always runs even if the KVDB delete errored, so the browser
// won't be left with a stale cookie.
func (m *SessionManager) DestroyUserSession(ctx context.Context, w http.ResponseWriter, sessionID string) error {
	err := m.DeleteUserSessionKVDB(ctx, sessionID)
	m.DeleteUserSessionCookie(w)
	return err
}

// DeleteUserSessionKVDB removes a user session's umbrella row and its entry in the
// per-user session list (if MaxSessionsPerUser > 0). Idempotent — no-op if the
// session is already gone.
func (m *SessionManager) DeleteUserSessionKVDB(ctx context.Context, sessionID string) error {
	row, err := m.FetchUserSession(ctx, sessionID)
	if err != nil {
		return err
	}
	if row == nil {
		return nil
	}

	if m.Conf.UserSession.MaxSessionsPerUser > 0 {
		usrSessionListKey := fmt.Sprintf("%s:cul:%s", m.appName, row.UID)
		_, _ = m.KVDB.ListRemove(ctx, usrSessionListKey, 0, sessionID)
	}

	_, _ = m.KVDB.Delete(ctx, m.UserSessionRowKey(sessionID))
	return nil
}

// ExtendUserSession extends both the KVDB-side TTL and the browser cookie's
// MaxAge for a user session. Best-effort — Expire silently no-ops if rows are
// missing.
func (m *SessionManager) ExtendUserSession(ctx context.Context, w http.ResponseWriter, sessionID, uidStr, encCookieValue string) {
	m.ExtendUserSessionKVDB(ctx, sessionID, uidStr)
	m.ExtendUserSessionCookie(w, encCookieValue)
}

// ExtendUserSessionKVDB extends the sliding TTL on the user session umbrella row
// and the per-user cap list row (if MaxSessionsPerUser > 0) using
// Conf.UserSession.ExpireIn. Best-effort.
func (m *SessionManager) ExtendUserSessionKVDB(ctx context.Context, sessionID, uidStr string) {
	m.ExtendUserSessionKVDBWithTTL(ctx, sessionID, uidStr,
		time.Duration(m.Conf.UserSession.ExpireIn)*time.Second)
}

// ExtendUserSessionKVDBWithTTL extends the TTL on the user session umbrella row
// and the per-user cap list row (if MaxSessionsPerUser > 0) by the given ttl.
// Best-effort — Expire calls silently no-op if the row doesn't exist.
func (m *SessionManager) ExtendUserSessionKVDBWithTTL(ctx context.Context, sessionID, uidStr string, ttl time.Duration) {
	_, _ = m.KVDB.Expire(ctx, m.UserSessionRowKey(sessionID), ttl)
	if m.Conf.UserSession.MaxSessionsPerUser > 0 {
		usrSessionListKey := fmt.Sprintf("%s:cul:%s", m.appName, uidStr)
		_, _ = m.KVDB.Expire(ctx, usrSessionListKey, ttl)
	}
}

// SetUserSessionCookie writes the Set-Cookie HTTP response header for a user
// session, with the sessionID encrypted via UserCookieCipher. MaxAge matches
// Conf.UserSession.ExpireIn. HttpOnly + Secure + SameSite=Lax.
func (m *SessionManager) SetUserSessionCookie(w http.ResponseWriter, sessionID string) error {
	encSessionID, err := m.UserCookieCipher.EncryptEncode([]byte(sessionID), m.UserCookieCipherContext())
	if err != nil {
		return fmt.Errorf("failed to encrypt user session id. %v", err)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     UserCookieName,
		Value:    encSessionID,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   m.Conf.UserSession.ExpireIn,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// ExtendUserSessionCookie resets the user session cookie's MaxAge using
// Conf.UserSession.ExpireIn. Same encrypted value, new lifetime.
func (m *SessionManager) ExtendUserSessionCookie(w http.ResponseWriter, encValue string) {
	m.ExtendUserSessionCookieWithMaxAge(w, encValue, m.Conf.UserSession.ExpireIn)
}

// ExtendUserSessionCookieWithMaxAge resets the user session cookie's MaxAge to
// the given seconds, keeping the same encrypted value.
func (m *SessionManager) ExtendUserSessionCookieWithMaxAge(w http.ResponseWriter, encValue string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     UserCookieName,
		Value:    encValue, // SAME encrypted value
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		MaxAge:   maxAge,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) DeleteUserSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     UserCookieName,
		Path:     "/",
		MaxAge:   -1, // Delete
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *SessionManager) VerifyUserSessionCookie(ctx context.Context, r *http.Request) *errs.Error {
	sessionCookie, err := r.Cookie(UserCookieName)
	if err != nil {
		return errs.CookieNotFound
	}
	cookieSessionID, err := m.UserCookieCipher.DecodeDecrypt(sessionCookie.Value, m.UserCookieCipherContext())
	if err != nil {
		return errs.InvalidCookie.WithCause(err)
	}
	found, err := m.UserSessionRowExists(ctx, string(cookieSessionID))
	if err != nil {
		return errs.KVDB.WithDetail("verify user session cookie").WithCause(err)
	}
	if !found {
		return errs.CookieSessionNotFound
	}
	return nil
}

// UserSessionIDFromCookie returns the session id the request's user cookie
// names: ok is false when there is no cookie or it does not decrypt. Decrypt
// only — no row is read, nothing is written; whether the session is live is
// not answered here.
func (m *SessionManager) UserSessionIDFromCookie(r *http.Request) (sessionID string, ok bool) {
	sessionCookie, err := r.Cookie(UserCookieName)
	if err != nil {
		return "", false
	}
	sessionIDBytes, err := m.UserCookieCipher.DecodeDecrypt(sessionCookie.Value, m.UserCookieCipherContext())
	if err != nil {
		return "", false
	}
	return string(sessionIDBytes), true
}

// UserLogoutRedirect clears the user cookie session (KVDB rows + cookie) and redirects
// to the given path. KVDB cleanup is best-effort; the redirect always proceeds.
// Caution: writes a redirect response to w; caller should not write to w afterward.
func (m *SessionManager) UserLogoutRedirect(w http.ResponseWriter, r *http.Request, redirectPath string) {
	if sessionID, ok := m.UserSessionIDFromCookie(r); ok {
		_ = m.DeleteUserSessionKVDB(r.Context(), sessionID)
	}
	m.DeleteUserSessionCookie(w)
	http.Redirect(w, r, redirectPath, http.StatusSeeOther)
}
