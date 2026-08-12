package handlerwrappers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/web/responses"
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
	ParseUID func(string) (UID, error)
}

func (m *CookieUserSession[UID]) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	sessSvc := appCore.SessionService
	if sessSvc == nil || sessSvc.CookieSessionManager == nil {
		log.Fatal("[ERROR] CookieUserSession - session manager missing")
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
		log.Fatal("[ERROR] invalid cookie session expiration mode")
		return nil
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
	})
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
		responses.WriteSimpleErrorJSON(w, http.StatusUnauthorized, fmt.Sprintf("invalid session. %v", err))
		return nil, nil, "", "", false
	}
	sessionID = string(sessionIDBytes)

	row, err := mgr.FetchUserSession(ctx, sessionID)
	if err != nil {
		responses.WriteSimpleErrorJSON(w, http.StatusInternalServerError, fmt.Sprintf("failed to check session. %v", err))
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
		responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.KVDB.WithDetail("parse uid").WithCause(err))
		return
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
