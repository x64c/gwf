package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/cookie"
)

// ActionLockCookieUser acquires named locks where each action targets either
// the cookie-session uid (sentinel "AuthUID") or a request path parameter.
//
// @field Actions map[string]string: { "actionName": "AuthUID" or "pathParamKey", ... }
type ActionLockCookieUser[UID comparable] struct {
	AppProvider framework.AppProviderFunc
	Actions     map[string]string
}

func (m *ActionLockCookieUser[UID]) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var uidStr string
		needsAuthUID := false
		for _, targetKey := range m.Actions {
			if targetKey == "AuthUID" {
				needsAuthUID = true
				break
			}
		}
		if needsAuthUID {
			sd, ok := cookie.UserSessionDataFromContext[UID](r.Context())
			if !ok {
				responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.DataMissingInContext.WithDetail("SessionData"))
				return
			}
			uidStr = sd.UIDStr
		}

		lockKeys := make([]string, 0, len(m.Actions))
		for action, targetKey := range m.Actions {
			var target string
			if targetKey == "AuthUID" {
				target = uidStr
			} else {
				target = r.PathValue(targetKey)
			}
			lockKeys = append(lockKeys, fmt.Sprintf("%s:%s", action, target))
		}
		runActionLocks(w, r, inner, appCore, lockKeys, uidStr)
	})
}
