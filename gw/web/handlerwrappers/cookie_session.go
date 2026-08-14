package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// CookieSession gates an endpoint on the cookie session protocol being in
// service. It rejects with 503 SessionServiceUnavailable when the protocol
// isn't serving — the session service is not admitted for use (its framework
// handle answers the lifecycle question), or the cookie protocol is disabled
// (the manager's own switch) — and otherwise passes the request through
// unchanged.
//
// It produces no SessionData. CookieUserSession is this gate plus user-identity
// resolution; use CookieSession on routes that depend on the cookie subsystem
// but carry no user identity — so a disabled/stopped protocol closes them up
// front instead of doing work that can't complete.
type CookieSession struct {
	AppProvider framework.AppProviderFunc
}

func (m *CookieSession) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	sessSvc := appCore.SessionService
	if sessSvc == nil || sessSvc.CookieSessionManager == nil {
		return nil, fmt.Errorf("CookieSession: cookie session manager missing — prepare it before wiring this middleware")
	}
	sessHandle := appCore.SessionHandle()
	mgr := sessSvc.CookieSessionManager
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessHandle.Get(); !ok || !mgr.Enabled() {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("cookie session"))
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}
