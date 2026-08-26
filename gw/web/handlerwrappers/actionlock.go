package handlerwrappers

import (
	"log"
	"net/http"
	"strings"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/namedlocks"
	"github.com/x64c/gwf/gw/web/responses"
)

// runActionLocks acquires the given locks; on conflict writes 409 and returns.
// On success, attaches acquired locks to ctx, runs inner, releases on defer.
// A panic from inner is logged with authUIDStr (empty if not auth-keyed) and
// raised again: the locks are released, the response stays the outer
// recovery's to decide.
// Used by ActionLockPathOnly, ActionLockBearerUser, and ActionLockCookieUser to share the lock-acquire logic.
func runActionLocks(w http.ResponseWriter, r *http.Request, inner http.Handler, appCore *framework.Core, lockKeys []string, authUIDStr string) {
	acquired, ok := namedlocks.AcquireLocks(appCore.ActionLocks, lockKeys)
	if !ok {
		// Fail-fast: resource is already locked
		if len(lockKeys) == 1 {
			responses.WriteErrorJSON(w, http.StatusConflict, errs.ActionLocked.WithDetail(lockKeys[0]))
			return
		}
		lockedActionsStr := strings.Join(lockKeys, ", ")
		responses.WriteErrorJSON(w, http.StatusConflict, errs.ActionLocked.WithDetail("some of "+lockedActionsStr))
		return
	}
	defer func() {
		namedlocks.ReleaseLocks(appCore.ActionLocks, acquired)
		if rcv := recover(); rcv != nil {
			// This frame recovers only to release the locks and to log what it
			// alone knows — the lock names and the acting user. It does not
			// answer the client: the panic is raised again so the outer
			// recovery keeps deciding what a panicked request looks like.
			// Without this, the recovered panic left the handler returning
			// normally and net/http replied 200 with an empty body.
			log.Printf("[PANIC] user=%s method=%s path=%s locks=%v err=%v",
				authUIDStr, r.Method, r.URL.Path, acquired, rcv)
			panic(rcv)
		}
	}()
	ctx := namedlocks.ContextWithAcquiredLocks(r.Context(), acquired)
	inner.ServeHTTP(w, r.WithContext(ctx))
}
