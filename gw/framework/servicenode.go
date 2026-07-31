package framework

import (
	"strings"
	"sync/atomic"

	"github.com/x64c/gwf/gw/svc"
)

// ServiceNode is Core's record of one registered service: the app-level verdict
// on whether that service may be used right now, plus the edges that decide
// start and teardown order.
//
// Two facts, two owners. The SERVICE owns its phase (svc.State) — only it can
// know whether its run goroutine exited or its listener closed. The NODE owns
// admission — a verdict derived from the phase plus graph facts the service
// does not have. They are not redundant, and during a degraded shutdown they
// legitimately disagree: the service still reports TERMINATING (true, it is
// still winding down) while Core has already stopped waiting for it (also
// true). One field could not hold both without one of them lying.
type ServiceNode struct {
	// Hot. Read on every gated use; written only at boot, at shutdown, and on
	// operator action — a textbook read-mostly variable.
	admitted atomic.Bool

	// Padding so a field added later cannot share this cache line with
	// `admitted` and invalidate it on every write. Every cold field below is
	// boot/shutdown-only today, so nothing contends yet; this keeps that true
	// by making the mistake take effort. Counters belong on another struct.
	_ [64]byte

	// Cold. Boot and shutdown only — never read on a request path.
	svc        svc.Service
	name       string
	idx        int           // registration index, so ordering passes need no lookup table
	declared   []ServiceDep  // what this node ASKED for, resolved or not — kept so an unresolved optional can still be answered for
	deps       []serviceEdge // what this node depends on; always registered earlier
	dependents []serviceEdge // reverse index, maintained at registration; each carries the kind THEY declared
	termSig    chan error    // buffered(1); this service's Terminated(), forwarded here so a barrier can wait for THIS node
	abandoned  bool          // Core stopped waiting for it during shutdown
}

// Name reports the node's display label, used by logs and status commands.
// Edges are resolved by pointer, never by name.
func (n *ServiceNode) Name() string { return n.name }

// Service reports the registered service. Reaching a service through a node
// bypasses the admission gate, so this is for Core and the control plane —
// consumers hold a Handle instead.
func (n *ServiceNode) Service() svc.Service { return n.svc }

// Admitted reports whether the app permits this service to be used. A service
// whose goroutine is alive can still be un-admitted: the process may be tearing
// down, or Core may have abandoned it.
func (n *ServiceNode) Admitted() bool { return n.admitted.Load() }

// Abandoned reports whether Core gave up waiting for this service to terminate.
// The service's own phase stays whatever it last set; this is Core's verdict
// alongside it, and it means the node is not restartable.
func (n *ServiceNode) Abandoned() bool { return n.abandoned }

// dependencyList renders this node's resolved dependencies for a boot log, so
// an app's composition is visible in the place people already read.
func (n *ServiceNode) dependencyList() string {
	var b strings.Builder
	for i, e := range n.deps {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(e.node.name)
		if e.optional {
			b.WriteString(" (may-use)")
		}
	}
	return b.String()
}

// neverAdmitted backs handles for an absent optional dependency, so Get needs
// no nil check on the hot path.
var neverAdmitted = &ServiceNode{name: "<absent>"}
