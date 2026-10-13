package udscmds

import (
	"fmt"
	"io"
	"slices"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/uds"
)

type BearerIssueTokenPair struct {
	AppProvider framework.AppProviderFunc
}

func (h *BearerIssueTokenPair) GroupName() string {
	return "bearer"
}

func (h *BearerIssueTokenPair) Command() string {
	return "bearer-issue-tokenpair"
}

func (h *BearerIssueTokenPair) Desc() string {
	return "Create a bearer session and print its sid and token pair; the session counts toward its group's cap"
}

func (h *BearerIssueTokenPair) Usage() string {
	return h.Command() + " client-id [user-id]"
}

func (h *BearerIssueTokenPair) HandleCommand(args []string, _ *uds.Prompter, w io.Writer) error {
	if len(args) < 1 || len(args) > 2 {
		return fmt.Errorf("usage: %s", h.Usage())
	}
	clientID := args[0]

	appCore := h.AppProvider().AppCore()
	// Running-plane access, as the bearer gates use: a session issued while the
	// service is down or disabled would be refused on its first request.
	sessSvc, ok := appCore.SessionHandle().Get()
	if !ok || sessSvc.BearerSessionManager == nil || !sessSvc.BearerSessionManager.Enabled() {
		return fmt.Errorf("bearer sessions not ready")
	}
	mgr := sessSvc.BearerSessionManager

	clientConf, ok := mgr.ClientConfs[clientID]
	if !ok {
		return fmt.Errorf("unknown client id %q", clientID)
	}
	group := clientConf.Group

	// Every group binds "client"; a group that also binds "user" needs the user id.
	bindValues := map[string]string{"client": clientID}
	if slices.Contains(group.Binds, "user") {
		if len(args) != 2 || args[1] == "" {
			return fmt.Errorf("group %q binds a user; usage: %s", group.Name, h.Usage())
		}
		bindValues["user"] = args[1]
	} else if len(args) == 2 {
		return fmt.Errorf("group %q binds no user", group.Name)
	}

	sid, accessToken, refreshToken, err := mgr.CreateSession(appCore.RootCtx, group, bindValues)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(w, "group: %s\nsid: %s\naccess: %s\nrefresh: %s\n", group.Name, sid, accessToken, refreshToken)
	return nil
}
