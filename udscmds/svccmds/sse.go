package svccmds

import (
	"fmt"
	"io"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/sse"
)

// SSE is the svc-cmd for the SSE service: the lifecycle verbs, and a status that adds the open streams and what has
// ended them.
type SSE struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "sse"
}

func (h *SSE) Name() string {
	if h.name == "" {
		return "sse"
	}
	return h.name
}

func (h *SSE) Rename(name string) { h.name = name }

func (h *SSE) Usage() string { return lifecycleUsage }

func (h *SSE) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	node := appCore.SSEHandle().Node()
	s, ok := node.Service().(*sse.Service)
	if !ok {
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	if err := doLifecycle(appCore, node, subcmd, args); err != nil {
		return err
	}
	printLifecycleStatus(node, w)
	st := s.Stats()
	_, _ = fmt.Fprintf(w, "  open streams: %d\n", st.StreamCount)
	_, _ = fmt.Fprintf(w, "  ended by frame write timeout (client not taking): %d\n", st.FrameWriteTimeoutCount)
	return nil
}
