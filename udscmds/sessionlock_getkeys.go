package udscmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/session"
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
	// Node-plane typed access: inspection must work on a stopped service too —
	// the lock manager is passive state and survives Stop.
	sessSvc, ok := appCore.SessionHandle().Node().Service().(*session.Service)
	if !ok || sessSvc.LockingManager() == nil {
		return fmt.Errorf("session locks not ready")
	}
	// Names only, as the manager reports them: what this instance holds now.
	for _, name := range sessSvc.LockingManager().Names() {
		_, _ = fmt.Fprintln(w, name)
	}
	return nil
}
