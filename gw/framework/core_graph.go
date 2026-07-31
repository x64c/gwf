package framework

import (
	"fmt"

	"github.com/x64c/gwf/gw/svc"
)

// serviceGraph is Core's registry of services and the dependencies between
// them.
//
// A DEPENDENCY here is one directed relation — "this service needs that one
// alive to do its job" — declared by the code that wires them together, and
// held once resolved as a pointer from the dependent to what it depends on,
// mirrored so each service also knows who depends on IT. (Graph vocabulary
// calls the services nodes and the relations edges; both words appear below.)
//
// This replaces a flat slice, whose only expressible relation was "also
// registered" — which is why a dependency could be torn down while something
// still using it was mid-teardown, with nothing able to state otherwise.
//
// The rule that keeps this small: A DEPENDENCY MAY ONLY NAME AN ALREADY-
// REGISTERED SERVICE. Registration is therefore forced into dependency order,
// and two things follow — a cycle is unwritable so no detection exists, and
// registration order is itself a valid topological order so no sort exists.
//
// DANGER: registration order being *valid* is not the guarantee. Correctness
// comes from waiting for each service's Terminated signal before touching what
// it depends on. Anyone who reads "we terminate in reverse order" and removes
// those waits reproduces the original defect exactly.
type serviceGraph struct {
	nodes  []*ServiceNode
	byName map[string]*ServiceNode // labels; boot and control plane only
	sealed bool                    // set by seal; registration is closed afterwards
}

// add registers a service together with what it depends on, and returns its
// node.
//
// The two kinds of dependency resolve at different moments, and the asymmetry
// is forced by what absence means rather than chosen:
//
//   - Requires is decided HERE, because an unknown label is always wrong. The
//     dependency must already be registered, so failing at the call puts the
//     error where the mistake is.
//   - MayUse cannot be decided here, because absence is legal — the app may
//     simply not have wired one. Its verdict waits for seal, the first moment
//     "never registered" can be told apart from "registered later".
//
// Every dependency that does resolve names an already-registered service, so
// every edge points backwards and the graph is acyclic by construction.
//
// The same label may be declared more than once, from different vantage
// points: a subsystem knowing its middleware might use a service, and a
// consumer inside it knowing that it must. Those collapse to one edge carrying
// the STRICTEST declaration, so a later question cannot depend on which
// declaration it happened to read first.
func (g *serviceGraph) add(s svc.Service, deps ...ServiceDep) (*ServiceNode, error) {
	if s == nil {
		return nil, fmt.Errorf("service graph: nil service")
	}
	if g.sealed {
		return nil, fmt.Errorf("service graph: %q registered after the graph was sealed — a service registered once the app is starting would never be started, ordered, or admitted", s.Name())
	}
	name := s.Name()
	if name == "" {
		return nil, fmt.Errorf("service graph: service registered with an empty name — the name identifies this instance in logs, in status output, and in every dependency declaration")
	}
	if _, dup := g.byName[name]; dup {
		return nil, fmt.Errorf("service graph: duplicate service name %q — names identify registered instances, so they must be unique", name)
	}

	n := &ServiceNode{svc: s, name: name, idx: len(g.nodes), declared: deps, termSig: make(chan error, 1)}
	for i, d := range deps {
		if d.label == "" {
			return nil, fmt.Errorf("service graph: %q declares a dependency with an empty label (dependency #%d of %d)", name, i+1, len(deps))
		}
		if d.label == name {
			return nil, fmt.Errorf("service graph: %q declares a dependency on itself", name)
		}
		target := g.byName[d.label]
		if target == nil {
			if d.optional {
				continue // verdict deferred to seal
			}
			return nil, fmt.Errorf("service graph: %q requires %q, which is not registered — a dependency must be registered before its dependent", name, d.label)
		}
		if j := indexOfEdge(n.deps, target); j >= 0 {
			n.deps[j].optional = n.deps[j].optional && d.optional // strictest wins
			continue
		}
		n.deps = append(n.deps, serviceEdge{node: target, optional: d.optional})
	}
	for _, e := range n.deps {
		e.node.dependents = append(e.node.dependents, serviceEdge{node: n, optional: e.optional})
	}

	if g.byName == nil {
		g.byName = make(map[string]*ServiceNode)
	}
	g.byName[name] = n
	g.nodes = append(g.nodes, n)
	return n, nil
}

// seal closes registration and settles what could not be settled earlier. Only
// unresolved MayUse labels are still open, and each has one of three answers:
//
//   - never registered — the app wired no such service. Correct, and silent:
//     the app chose this, so there is nothing to report.
//   - registered BEFORE the dependent — already wired at registration.
//   - registered AFTER the dependent — an ordering mistake, and the reason
//     this check exists. The edge cannot be created, since it would point
//     forward and break the property that makes this graph sort-free; and it
//     cannot be ignored, since that is the silent miss the whole design exists
//     to prevent. So it fails the boot.
//
// That third case is detectable only because a dependency is declared by NAME.
// A nil pointer carries no identity, so the same mistake would leave the graph
// internally consistent and simply missing an edge nobody knew was wanted.
func (g *serviceGraph) seal() error {
	g.sealed = true
	for _, n := range g.nodes {
		for _, d := range n.declared {
			if !d.optional {
				continue // settled at registration
			}
			if indexOfEdgeByName(n.deps, d.label) >= 0 {
				continue // wired
			}
			if late := g.byName[d.label]; late != nil {
				return fmt.Errorf("service graph: %q declares MayUse(%q), which registered later (position %d, after %d) — register a service before anything that may use it", n.name, d.label, late.idx, n.idx)
			}
		}
	}
	return nil
}

// indexOfEdge reports where an edge to n sits, or -1. Linear because the list
// it searches is one service's own dependencies — a handful, fixed when the
// call is written — not the registry, which is keyed.
func indexOfEdge(edges []serviceEdge, n *ServiceNode) int {
	for i := range edges {
		if edges[i].node == n {
			return i
		}
	}
	return -1
}

// indexOfEdgeByName is indexOfEdge where only the label is known, because the
// node was never resolved.
func indexOfEdgeByName(edges []serviceEdge, label string) int {
	for i := range edges {
		if edges[i].node.name == label {
			return i
		}
	}
	return -1
}

// find returns the node registered under a name, or nil. Boot and control
// plane only: nothing on a request path may reach it.
func (g *serviceGraph) find(name string) *ServiceNode { return g.byName[name] }

// levels groups the nodes so that every node sits one step behind everything
// that depends on it: level 0 holds the nodes nothing depends on, level 1 the
// nodes whose dependents are all in level 0, and so on.
//
// One grouping serves both ends of the lifecycle, walked in opposite
// directions — TERMINATE ascending (dependents die before what they depend on)
// and START descending (dependencies run before what depends on them). Within
// a level the nodes are independent, so they can proceed concurrently: nothing
// in the graph says one must wait for the other, and imposing a wait anyway
// would both slow shutdown and assert a relationship that does not exist.
//
// One backward pass suffices because every edge points backwards: a node's
// dependents are always registered after it, so their levels are already
// computed by the time it is reached.
func (g *serviceGraph) levels() [][]*ServiceNode {
	if len(g.nodes) == 0 {
		return nil
	}
	lvl := make([]int, len(g.nodes))
	highest := 0
	for i := len(g.nodes) - 1; i >= 0; i-- {
		for _, e := range g.nodes[i].dependents {
			if l := lvl[e.node.idx] + 1; l > lvl[i] {
				lvl[i] = l
			}
		}
		if lvl[i] > highest {
			highest = lvl[i]
		}
	}
	out := make([][]*ServiceNode, highest+1)
	for i, n := range g.nodes {
		out[lvl[i]] = append(out[lvl[i]], n)
	}
	return out
}
