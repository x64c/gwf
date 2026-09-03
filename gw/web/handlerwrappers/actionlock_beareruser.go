package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
	"github.com/x64c/gwf/gw/web/session/bearer"
)

// ActionLockBearerUser acquires named locks where each action targets either
// the bearer-session uid (sentinel "AuthUID") or a request path parameter.
//
// @field Actions map[string]string: { "actionName": "AuthUID" or "pathParamKey", ... }
type ActionLockBearerUser[UID comparable] struct {
	AppProvider framework.AppProviderFunc
	Actions     map[string]string
}

func (m *ActionLockBearerUser[UID]) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	if err := requireActionLocks(appCore, "ActionLockBearerUser"); err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Look up session only if any action needs the AuthUID.
		var uidStr string
		needsAuthUID := false
		for _, targetKey := range m.Actions {
			if targetKey == "AuthUID" {
				needsAuthUID = true
				break
			}
		}
		if needsAuthUID {
			sd, ok := bearer.UserSessionDataFromContext[UID](r.Context())
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
	}), nil
}
