package handlerwrappers

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

type CookieUserSession[UID comparable] struct {
	AppProvider framework.AppProviderFunc
	// ParseUID converts the stored identity into the app's UID type. What is
	// stored is always a string — that is what a KVDB holds — so this is where
	// a string becomes an identity.
	//
	// It MUST return an error for any string that does not name one, the empty
	// string included. Nothing else can make that judgement: only the app knows
	// what its UID type admits, and a ParseUID that accepts "" turns a session
	// carrying no identity into an authenticated principal.
	//
	// The middleware deliberately does NOT pre-check "" before calling this.
	// ParseUID must already reject it — apps call ParseUID from their own
	// sites too, so it can never assume a sanitized caller — and a
	// middleware-side guard would therefore be a second check of the same
	// string on every authenticated request, while teaching parser authors
	// that someone else handles the empty case. There is one judge, it is
	// this function, and its contract is total. (A parse failure is answered
	// as an invalid session — cookie cleared, redirect to login — never a
	// bare error page; see authenticateCookieSession.)
	ParseUID func(string) (UID, error)
}

func (m *CookieUserSession[UID]) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	// Wrap runs at boot, before StartServices admits anything, so the manager
	// is reached on the NODE plane — the handle would refuse Get() here. The
	// per-request gate below still asks the handle.
	sessSvc, _ := appCore.SessionHandle().Node().Service().(*session.Service)
	if sessSvc == nil || sessSvc.CookieSessionManager == nil {
		return nil, fmt.Errorf("CookieUserSession: cookie session manager missing — prepare it before wiring this middleware")
	}
	sessHandle := appCore.SessionHandle()
	mgr := sessSvc.CookieSessionManager

	var authHandler http.Handler
	switch mgr.Conf.UserSession.ExpireMode {
	case cookie.ExpireAbsolute:
		authHandler = m.absoluteExpHandler(inner, mgr)
	case cookie.ExpireSliding:
		authHandler = m.slidingExpHandler(inner, mgr)
	default:
		// Unreachable with valid config — ExpireMode is validated at PrepareCookieSessions.
		return nil, fmt.Errorf("CookieUserSession: invalid cookie session expiration mode %q", mgr.Conf.UserSession.ExpireMode)
	}

	// Gate: reject when the cookie protocol isn't serving. Lifecycle comes from
	// the framework handle (un-admitted = stopped, terminating, or never
	// wired); the protocol's own on/off switch stays the manager's. Additive
	// protocol → can't attach identity → 503.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessHandle.Get(); !ok || !mgr.Enabled() {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("cookie user session"))
			return
		}
		authHandler.ServeHTTP(w, r)
	}), nil
}

func (m *CookieUserSession[UID]) authenticateCookieSession(
	w http.ResponseWriter, r *http.Request, mgr *cookie.SessionManager,
) (
	ctx context.Context, sessionCookie *http.Cookie, sessionID string, uidStr string, ok bool, // ok to proceed
) {
	ctx = r.Context()
	sessionCookie, err := r.Cookie(cookie.UserCookieName)
	if err != nil { // http.ErrNoCookie
		// Session Cookie Not Found = Non-login Hit to Auth-protected Endpoints
		// Redirect to Login page setting Intended URI Cookie
		cookie.SetIntendedURICookie(w, r, 60) // short-lived cookie
		http.Redirect(w, r, mgr.Conf.UserSession.LoginPath+"?endpoint=protected", http.StatusSeeOther)
		return nil, nil, "", "", false
	}
	sessionIDBytes, err := mgr.UserCookieCipher.DecodeDecrypt(sessionCookie.Value, mgr.UserCookieCipherContext())
	if err != nil {
		// The cookie names no readable session (garbage, or sealed under a
		// retired key). Answered like any other invalid session: end it and
		// send the human back to login. A bare error here would strand the
		// browser — the cookie would be re-presented on every request.
		mgr.DeleteUserSessionCookie(w)
		cookie.SetIntendedURICookie(w, r, 60)
		http.Redirect(w, r, mgr.Conf.UserSession.LoginPath+"?session=invalid", http.StatusSeeOther)
		return nil, nil, "", "", false
	}
	sessionID = string(sessionIDBytes)

	row, err := mgr.FetchUserSession(ctx, sessionID)
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("failed to check session").WithCause(err))
		return nil, nil, "", "", false
	}
	if row == nil {
		// Session Not Found. Session might have been Expired.
		// Redirect to Login page Clearing Session Cookie
		mgr.DeleteUserSessionCookie(w)
		cookie.SetIntendedURICookie(w, r, 60)
		http.Redirect(w, r, mgr.Conf.UserSession.LoginPath+"?session=expired", http.StatusSeeOther)
		return nil, nil, "", "", false
	}
	uidStr = row.UID
	csrfTkn := row.CSRF

	uid, err := m.ParseUID(uidStr)
	if err != nil {
		// The row exists but its identity does not parse — a corrupt session,
		// not a server fault. Answered like any other invalid session: end it
		// (row + cookie) and send the human back to login. A bare error here
		// would strand the browser in a loop — the cookie would re-present
		// the same corrupt row on every request with no way back.
		_ = mgr.DeleteUserSessionKVDB(ctx, sessionID)
		mgr.DeleteUserSessionCookie(w)
		cookie.SetIntendedURICookie(w, r, 60)
		http.Redirect(w, r, mgr.Conf.UserSession.LoginPath+"?session=invalid", http.StatusSeeOther)
		return nil, nil, "", "", false
	}

	ctx = cookie.WithUserSessionData(ctx, &cookie.UserSessionData[UID]{
		ID:      sessionID,
		UIDStr:  uidStr,
		UID:     uid,
		CSRFTkn: csrfTkn,
		Mgr:     mgr,
	})
	return ctx, sessionCookie, sessionID, uidStr, true
}

func (m *CookieUserSession[UID]) absoluteExpHandler(inner http.Handler, mgr *cookie.SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, _, _, _, ok := m.authenticateCookieSession(w, r, mgr)
		if !ok {
			return
		}
		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (m *CookieUserSession[UID]) slidingExpHandler(inner http.Handler, mgr *cookie.SessionManager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, sessionCookie, sessionID, uidStr, ok := m.authenticateCookieSession(w, r, mgr)
		if !ok {
			return
		}

		ttl, state, err := mgr.KVDB.TTL(ctx, mgr.UserSessionRowKey(sessionID))
		if err == nil && state == kvdbs.TTLExpiring && ttl < time.Duration(mgr.Conf.UserSession.ExtendThreshold)*time.Second {
			mgr.ExtendUserSession(ctx, w, sessionID, uidStr, sessionCookie.Value)
		}

		inner.ServeHTTP(w, r.WithContext(ctx))
	})
}
