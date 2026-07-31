package framework

// ServiceDep is one declared dependency: the LABEL of the service depended on,
// plus whether the dependent can carry on without it.
//
// Dependencies are declared by label rather than by pointer for one reason:
// a label survives failing to resolve. An optional dependency that is not
// registered yet leaves nothing behind if it was a nil pointer — the graph ends
// up internally consistent, simply missing an edge nobody knows was wanted. A
// label can be remembered, so the same situation becomes a question the graph
// can answer at seal.
//
// The two kinds differ in exactly ONE thing — what an unresolved label means.
// Ordering and lifetime guarantees are identical for both: a dependency starts
// before anything that depends on it, and is torn down only after every
// dependent has fully terminated, optional or not. A service that only MayUse
// another can still be mid-call when that other one dies.
//
// They are also different in KIND, not merely in strength. Requires covers a
// strict prerequisite — something that must be up and must outlive this
// service, whether or not it is ever called: a migration that must have run, a
// hook that must be installed. MayUse exists only because something inside
// might call it; if nothing ever did, there would be no reason to declare it.
type ServiceDep struct {
	label    string
	optional bool
}

// Requires declares a strict dependency: it must already be registered, and it
// must outlive this service. Whether this service ever calls it is its own
// business — a prerequisite that is never invoked is still a dependency.
//
// Resolved at the registration call: an unknown label fails there and then,
// naming the dependent and what it asked for.
func Requires(label string) ServiceDep { return ServiceDep{label: label} }

// MayUse declares that code inside the registering service might call another
// service — so if the app wired one, it must be ordered as a dependency. It is
// a declaration of a POSSIBILITY, not of a use: a subsystem rarely knows
// whether any of the middleware or handlers mounted inside it actually reaches
// for the other service, only that they could.
//
// Unresolved at registration is not an error, because absence is legal: the app
// is the only thing that knows what it wired. The verdict is deferred to seal,
// where three outcomes are distinguished — never registered (fine, silent),
// registered earlier (wired), registered LATER (an ordering mistake, and the
// case that would otherwise vanish without trace).
//
// Legitimate when the no-dependency path is honest degradation — a cache that
// falls back to its source, a metrics sink that skips, a best-effort audit
// trail. If the absence weakens a guarantee instead of costing a convenience,
// it is Requires.
func MayUse(label string) ServiceDep { return ServiceDep{label: label, optional: true} }

// ServiceDeps is implemented by a service that knows its own dependencies —
// typically because it was constructed with them, so the edges are derived from
// the wiring that actually happened rather than from a parallel declaration
// someone has to remember to keep in step.
//
// RegisterService asks every service it registers. A service with no
// dependencies, or one whose dependencies are held by things it never sees
// (middleware, handlers), simply does not implement it; the caller declares
// those instead.
type ServiceDeps interface {
	ServiceDeps() []ServiceDep
}

// Label reports the name of the service depended on.
func (d ServiceDep) Label() string { return d.label }

// Optional reports whether the dependent declared it can work without this
// dependency, i.e. MayUse rather than Requires.
func (d ServiceDep) Optional() bool { return d.optional }

// serviceEdge is a resolved dependency: the node at the other end, and the kind
// it was declared with. On a node's own deps the node is what it depends on; on
// its dependents, what depends on it — carrying the kind THEY declared, which
// is what decides whether they may block a stop.
type serviceEdge struct {
	node     *ServiceNode
	optional bool
}
