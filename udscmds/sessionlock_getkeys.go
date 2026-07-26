package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/session/lockstore"
)

type SessionlockGetKeys struct {
	AppProvider framework.AppProviderFunc
}

func (h *SessionlockGetKeys) GroupName() string {
	return "sessionlock"
}

func (h *SessionlockGetKeys) Command() string {
	return "sessionlock-get-keys"
}

func (h *SessionlockGetKeys) Desc() string {
	return "Print session lock keys"
}

func (h *SessionlockGetKeys) Usage() string {
	return h.Command()
}

func (h *SessionlockGetKeys) HandleCommand(_ []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	sessionLocks := appCore.SessionService.SessionLocks
	if sessionLocks == nil {
		return fmt.Errorf("session locks not ready")
	}
	sessionLocks.Range(func(key string, _ *lockstore.LockEntry) bool {
		_, _ = fmt.Fprintln(w, key)
		return true
	})
	return nil
}
