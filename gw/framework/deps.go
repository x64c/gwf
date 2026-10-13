package framework

import (
	"fmt"
	"reflect"
	"slices"

	"github.com/x64c/gwf/gw/svc"
)

// Deps is what a factory receives (RegisterServiceFunc, PrepareWebService): the dependencies its registration
// declared, resolved against the services already registered. A handle can be taken only for a declared label, so a
// handle exists only where the graph has the edge — "used but never declared" cannot be written.
type Deps struct {
	name     string        // the service being registered (error messages)
	declared []ServiceDep  // as the registration declared them
	edges    []serviceEdge // the declarations that resolved: every Requires; a MayUse already registered
}

// Handle returns the gated handle to a dependency this registration declared. A MayUse dependency the app never
// registered gets a handle whose Get reports unavailable forever, so the no-service path and the service-unavailable
// path are the same code. On error the returned handle is that same never-available one, never a zero value.
//
// Take handles inside the factory and keep them; ask Get per use.
func (d Deps) Handle[T svc.Service](label string) (ServiceHandle[T], error) {
	if !slices.ContainsFunc(d.declared, func(dep ServiceDep) bool { return dep.label == label }) {
		return absentServiceHandle[T](), fmt.Errorf("service graph: %q took a handle to %q without declaring it — add Requires(%q) or MayUse(%q) to its registration", d.name, label, label, label)
	}
	i := indexOfEdgeByName(d.edges, label)
	if i < 0 {
		return absentServiceHandle[T](), nil // MayUse, not registered
	}
	n := d.edges[i].node
	s, ok := n.svc.(T)
	if !ok {
		return absentServiceHandle[T](), fmt.Errorf("service graph: %q took a %v handle to %q, which is a %T", d.name, reflect.TypeFor[T](), label, n.svc)
	}
	return newServiceHandle(n, s), nil
}
