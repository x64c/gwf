package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// BearerClient validates the "Client-Id" header against the bearer-session
// ClientConfs map. Rejects requests with missing or unregistered Client-Id.
// Optionally restricts to specific bearer-session groups via AllowedGroups
// (empty = accept any group from config).
//
// Use this on PUBLIC or AUTH-FLOW endpoints (login, OAuth callback, JWKS, etc.)
// that need to verify the calling client BEFORE a bearer token is available.
//
// Do NOT combine with BearerUserSession on authenticated routes — BearerUserSession
// already validates the client implicitly: the bearer token resolves to a session,
// and the session can only exist for a registered client. Adding BearerClient on
// top would be redundant.
type BearerClient struct {
	AppProvider   framework.AppProviderFunc
	AllowedGroups []string // empty = accept any group from config
}

func (m *BearerClient) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	sessSvc := appCore.SessionService
	if sessSvc == nil || sessSvc.BearerSessionManager == nil {
		return nil, fmt.Errorf("BearerClient: bearer session manager missing — prepare it before wiring this middleware")
	}
	mgr := sessSvc.BearerSessionManager

	// Boot-time: build allowed-groups lookup map once, captured in closure.
	var allowedGroupsSet map[string]struct{}
	if len(m.AllowedGroups) == 0 {
		allowedGroupsSet = make(map[string]struct{}, len(mgr.GroupConfs))
		for groupName := range mgr.GroupConfs {
			allowedGroupsSet[groupName] = struct{}{}
		}
	} else {
		allowedGroupsSet = make(map[string]struct{}, len(m.AllowedGroups))
		for _, g := range m.AllowedGroups {
			allowedGroupsSet[g] = struct{}{}
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientID := r.Header.Get("Client-Id")
		if clientID == "" {
			responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.BearerClientNotFound.WithDetail("missing Client-Id header"))
			return
		}
		clientConf, ok := mgr.ClientConfs[clientID]
		if !ok {
			responses.WriteErrorJSON(w, http.StatusUnauthorized, errs.BearerClientNotFound.WithDetail(fmt.Sprintf("unknown client_id: %s", clientID)))
			return
		}
		if _, ok := allowedGroupsSet[clientConf.Group.Name]; !ok {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.BearerSessionGroupNotAllowed.WithDetail(clientConf.Group.Name))
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}
