package handlerwrappers

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// BearerSession gates an endpoint on the bearer session protocol being in
// service. It rejects with 503 SessionServiceUnavailable when the protocol
// isn't serving — the session service is not admitted for use (its framework
// handle answers the lifecycle question), or the bearer protocol is disabled
// (the manager's own switch) — and otherwise passes the request through
// unchanged.
//
// It produces no SessionData. BearerUserSession is this gate plus user-identity
// resolution; use BearerSession on routes that depend on the bearer subsystem
// but carry no user identity — so a disabled/stopped protocol closes them up
// front instead of doing work that can't complete.
type BearerSession struct {
	AppProvider framework.AppProviderFunc
}

func (m *BearerSession) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	sessSvc := appCore.SessionService
	if sessSvc == nil || sessSvc.BearerSessionManager == nil {
		log.Fatal("[ERROR] BearerSession - session manager missing")
	}
	sessHandle := appCore.SessionHandle()
	mgr := sessSvc.BearerSessionManager
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessHandle.Get(); !ok || !mgr.Enabled() {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("bearer session"))
			return
		}
		inner.ServeHTTP(w, r)
	})
}
