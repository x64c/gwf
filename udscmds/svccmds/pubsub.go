package svccmds

import (
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/pubsubs"
)

// PubSub is the svc-cmd for the pub/sub service: the lifecycle verbs, and a status that adds its subscriptions.
type PubSub struct {
	AppProvider framework.AppProviderFunc
	name        string // optional override; defaults to "pubsub"
}

func (h *PubSub) Name() string {
	if h.name == "" {
		return "pubsub"
	}
	return h.name
}

func (h *PubSub) Rename(name string) { h.name = name }

func (h *PubSub) Usage() string { return lifecycleUsage }

func (h *PubSub) Handle(subcmd string, args []string, w io.Writer) error {
	appCore := h.AppProvider().AppCore()
	node := appCore.PubSubHandle().Node()
	b, ok := node.Service().(pubsubs.Broker)
	if !ok {
		return fmt.Errorf("%s service not configured in this app", h.Name())
	}
	if err := doLifecycle(appCore, node, subcmd, args); err != nil {
		return err
	}
	printLifecycleStatus(node, w)
	st := b.Stats()
	_, _ = fmt.Fprintf(w, "  subscriptions: %d\n", st.SubscriptionCount)
	topics := make([]string, 0, len(st.SubscriptionCountPerTopic))
	for t := range st.SubscriptionCountPerTopic {
		topics = append(topics, t)
	}
	slices.Sort(topics)
	parts := make([]string, 0, len(topics))
	for _, t := range topics {
		parts = append(parts, fmt.Sprintf("%s=%d", t, st.SubscriptionCountPerTopic[t]))
	}
	_, _ = fmt.Fprintf(w, "  per topic: %s\n", strings.Join(parts, " "))
	return nil
}
