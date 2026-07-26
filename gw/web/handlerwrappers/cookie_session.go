package handlerwrappers

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// CookieSession gates an endpoint on the cookie session protocol being in
// service. It rejects with 503 SessionServiceUnavailable when the protocol
// isn't serving — the session service is stopped, or the cookie protocol is
// disabled — and otherwise passes the request through unchanged.
//
// It produces no SessionData. CookieUserSession is this gate plus user-identity
// resolution; use CookieSession on routes that depend on the cookie subsystem
// but carry no user identity — so a disabled/stopped protocol closes them up
// front instead of doing work that can't complete.
type CookieSession struct {
	AppProvider framework.AppProviderFunc
}

func (m *CookieSession) Wrap(inner http.Handler) http.Handler {
	sessSvc := m.AppProvider().AppCore().SessionService
	if sessSvc == nil || sessSvc.CookieSessionManager == nil {
		log.Fatal("[ERROR] CookieSession - session manager missing")
	}
	mgr := sessSvc.CookieSessionManager
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !mgr.Serving() {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("cookie session"))
			return
		}
		inner.ServeHTTP(w, r)
	})
}
