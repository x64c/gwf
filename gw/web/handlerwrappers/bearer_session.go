package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session"
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

func (m *BearerSession) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	// Wrap runs at boot, before StartServices admits anything, so the manager
	// is reached on the NODE plane — the handle would refuse Get() here. The
	// per-request gate below still asks the handle.
	sessSvc, _ := appCore.SessionHandle().Node().Service().(*session.Service)
	if sessSvc == nil || sessSvc.BearerSessionManager == nil {
		return nil, fmt.Errorf("BearerSession: bearer session manager missing — prepare it before wiring this middleware")
	}
	sessHandle := appCore.SessionHandle()
	mgr := sessSvc.BearerSessionManager
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := sessHandle.Get(); !ok || !mgr.Enabled() {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.SessionServiceUnavailable.WithDetail("bearer session"))
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}
