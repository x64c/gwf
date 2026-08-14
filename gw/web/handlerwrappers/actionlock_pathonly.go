package handlerwrappers

import (
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/framework"
)

// ActionLockPathOnly acquires named locks for path-keyed actions.
// Each lock's name is "actionName:targetValue". The target value comes from
// r.PathValue(targetKey).
//
// @field Actions map[string]string: { "actionName":"pathParamKey", ... }
//
// For user-keyed locks (with optional path mix), see ActionLockBearerUser /
// ActionLockCookieUser instead.
type ActionLockPathOnly struct {
	AppProvider framework.AppProviderFunc
	Actions     map[string]string
}

func (m *ActionLockPathOnly) Wrap(inner http.Handler) (http.Handler, error) {
	appCore := m.AppProvider().AppCore()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lockKeys := make([]string, 0, len(m.Actions))
		for action, targetKey := range m.Actions {
			lockKeys = append(lockKeys, fmt.Sprintf("%s:%s", action, r.PathValue(targetKey)))
		}
		runActionLocks(w, r, inner, appCore, lockKeys, "")
	}), nil
}
