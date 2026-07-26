package handlerwrappers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/namedlocks"
	"github.com/x64c/gwf/gw/web/responses"
)

// runActionLocks acquires the given locks; on conflict writes 409 and returns.
// On success, attaches acquired locks to ctx, runs inner, releases on defer.
// authUIDStr is included in panic logs (empty if not auth-keyed).
// Used by ActionLockPathOnly, ActionLockBearerUser, and ActionLockCookieUser to share the lock-acquire logic.
func runActionLocks(w http.ResponseWriter, r *http.Request, inner http.Handler, appCore *framework.Core, lockKeys []string, authUIDStr string) {
	acquired, ok := namedlocks.AcquireLocks(appCore.ActionLocks, lockKeys)
	if !ok {
		// Fail-fast: resource is already locked
		if len(lockKeys) == 1 {
			responses.WriteSimpleErrorJSON(w, http.StatusConflict, fmt.Sprintf("action [%s] locked by another request", lockKeys[0]))
			return
		}
		lockedActionsStr := strings.Join(lockKeys, ", ")
		responses.WriteSimpleErrorJSON(w, http.StatusConflict, fmt.Sprintf("some of actions in [%s] locked by another request", lockedActionsStr))
		return
	}
	defer func() {
		namedlocks.ReleaseLocks(appCore.ActionLocks, acquired)
		if rcv := recover(); rcv != nil {
			log.Printf("[PANIC] user=%s method=%s path=%s locks=%v err=%v",
				authUIDStr, r.Method, r.URL.Path, acquired, rcv)
		}
	}()
	ctx := namedlocks.ContextWithAcquiredLocks(r.Context(), acquired)
	inner.ServeHTTP(w, r.WithContext(ctx))
}
