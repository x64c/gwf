package handlerwrappers

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/locking"
	"github.com/x64c/gwf/gw/web/responses"
)

// requireActionLocks reports whether the app supplied a lock manager for its
// action locks, naming the wrapper that needs one. It is asked at Wrap time:
// routes are built at boot, so a route whose locks the app never prepared
// fails the boot instead of dereferencing nil on the first request to reach
// it — the same shape as Throttle's wrap-time bucket-group check.
func requireActionLocks(appCore *framework.Core, wrapper string) error {
	if appCore.ActionLockingManager() == nil {
		return fmt.Errorf("%s: no action locks — call Core.PrepareActionLocks before building routes", wrapper)
	}
	return nil
}

// runActionLocks runs inner while every one of lockKeys is held, with the
// names attached to the request ctx; on refusal it writes 409 and returns.
// The manager releases the names when inner returns, by any path. A panic
// from inner is logged with authUIDStr (empty if not auth-keyed) and raised
// again: the names are released, the response stays the outer recovery's to
// decide.
// Used by ActionLockPathOnly, ActionLockBearerUser, and ActionLockCookieUser to share the lock-acquire logic.
func runActionLocks(w http.ResponseWriter, r *http.Request, inner http.Handler, appCore *framework.Core, lockKeys []string, authUIDStr string) {
	err := appCore.ActionLockingManager().AcquireDoReleaseAll(r.Context(), lockKeys, func(ctx context.Context) error {
		defer func() {
			if rcv := recover(); rcv != nil {
				// This frame recovers only to log what it alone knows — the
				// lock names and the acting user. It does not answer the
				// client: the panic is raised again so the outer recovery
				// keeps deciding what a panicked request looks like. Without
				// this, the recovered panic left the handler returning
				// normally and net/http replied 200 with an empty body.
				log.Printf("[PANIC] user=%s method=%s path=%s locks=%v err=%v",
					authUIDStr, r.Method, r.URL.Path, lockKeys, rcv)
				panic(rcv)
			}
		}()
		inner.ServeHTTP(w, r.WithContext(locking.ContextWithAcquiredLocks(ctx, lockKeys)))
		return nil
	})
	if err != nil {
		// Fail-fast: some name is already held (errs.ActionLocked names it),
		// the request ctx had ended, or the manager could not answer.
		responses.WriteErrorJSON(w, http.StatusConflict, errs.AsStructured(err, errs.ActionLocked))
	}
}
