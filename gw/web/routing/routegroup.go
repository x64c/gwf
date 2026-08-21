package routing

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/x64c/gwf/gw/web"
)

type RouteGroup struct {
	Router          // [Embedded]
	Prefix          string
	HandlerWrappers []web.HandlerWrapper // Group Handler Wrappers
}

// Ensure RouteGroup implements Router
var _ Router = (*RouteGroup)(nil)

// Handle registers a route pattern
func (g *RouteGroup) Handle(subpattern string, handler http.Handler, handlerWrappers ...web.HandlerWrapper) {
	var (
		subPatternParts []string
		subpath         string
		method          string
		fullPattern     string
	)

	subPatternParts = strings.SplitN(subpattern, " ", 2)
	if len(subPatternParts) == 2 {
		// subpattern "<method> <subpath>" -> fullpattern "<method> <groupPrefix><subpath>"
		// method: e.g. GET, POST
		method = subPatternParts[0]
		subpath = subPatternParts[1]
		fullPattern = method + " " + g.Prefix + subpath
	} else {
		fullPattern = g.Prefix + subpattern
	}

	if strings.Contains(fullPattern, "//") {
		panic(fmt.Sprintf("routing: can't register router pattern %q: empty path segment", fullPattern))
	}

	// Wrapping the Handler (Nesting) by the HandlerWrappers into the Actual Handler
	// Wrapped Handler = grpHndWrapr1 (
	//						<Group-PreAction1>
	//						...
	//						grpHndWraprN (
	//							<Group-PreActionN>
	//							hndWrapr1 (
	//								<Individual-PreAction1>
	//								...
	//								hndWraprN (
	//									<Individual-PreActionN>
	//									handler
	//									<Individual-PostActionN>
	//								)
	//								...
	//								<Individual-PostAction1>
	// 							)
	//							<Group-PostActionN>
	//						)
	//						...
	//						<Group-PostAction1>
	//					)
	// 1. Pre-action order:
	//		grpHndWrapr1 -> ... -> grpHndWraprN -> hndWrapr1 -> ... -> hndWraprN
	// 2. handler.ServeHTTP(w,r)
	// 3. Post-action order:
	//		grpHndWrapr1 <- ... <- grpHndWraprN <- hndWrapr1 <- ... <- hndWraprN
	// Wrappers are built inner-first (the loops count down), so a failure here
	// stops the chain before the wrappers outside it run — they would be
	// building against state the failed one was supposed to establish.
	//
	// A failure PANICS, by design. Handle cannot return an error — a route
	// table is a list of declarations, not a sequence of checked calls — and
	// the routes are declared at boot, before anything listens, so this is
	// fail-fast at the exact moment the mistake is made. Panic rather than
	// log.Fatal: stdlib parity (http.ServeMux.Handle panics on a bad pattern),
	// defers run, a host can recover, and the path is testable.
	wrappedHandler := handler
	// inline slices.Backward
	for i := len(handlerWrappers) - 1; i >= 0; i-- {
		wrapped, err := handlerWrappers[i].Wrap(wrappedHandler)
		if err != nil {
			panic(fmt.Sprintf("routing: route %q: %v", fullPattern, err))
		}
		wrappedHandler = wrapped
	}
	// inline slices.Backward
	for i := len(g.HandlerWrappers) - 1; i >= 0; i-- {
		wrapped, err := g.HandlerWrappers[i].Wrap(wrappedHandler)
		if err != nil {
			panic(fmt.Sprintf("routing: route %q (group %q): %v", fullPattern, g.Prefix, err))
		}
		wrappedHandler = wrapped
	}
	// Register the fullPattern with the WrappedHandler
	g.Router.Handle(fullPattern, wrappedHandler)
}

func (g *RouteGroup) HandleFunc(subpattern string, handleFunc func(http.ResponseWriter, *http.Request), handlerWrappers ...web.HandlerWrapper) {
	g.Handle(subpattern, http.HandlerFunc(handleFunc), handlerWrappers...)
}

// Group on *RouteGroup makes a Subgroup
//
//	router.Group("/foo/", func(foo *RouteGroup) {        // RouteGroup for "/foo/..."
//	  foo.Handle("GET bar", foobarGetHandler)            // "GET /foo/bar"
//
//	  foo.Group("baz/", func(foobaz *RouteGroup) {		 // RouteGroup for "/foo/baz/..." = Subgroup of "/foo/"
//	    foobaz.Handle("GET baas", foobazbaasGetHandler)  // "GET /foo/baz/baas"
//	    foobaz.Handle("POST bam", foobazbamPostHandler)  // "POST /foo/baz/bam"
//	  }
//	}
func (g *RouteGroup) Group(subPrefix string, batch func(*RouteGroup), handlerWrappers ...web.HandlerWrapper) *RouteGroup {
	// The subgroup OWNS its wrapper list — a fresh array, never a view into the
	// parent's. Appending onto the parent's slice let two sibling subgroups
	// write the same backing-array slot, so a route registered on a retained
	// subgroup after a sibling existed got the sibling's wrapper.
	wrappers := make([]web.HandlerWrapper, 0, len(g.HandlerWrappers)+len(handlerWrappers))
	wrappers = append(wrappers, g.HandlerWrappers...)
	wrappers = append(wrappers, handlerWrappers...)

	subg := &RouteGroup{
		Router:          g.Router,             // same router
		Prefix:          g.Prefix + subPrefix, // extended prefix
		HandlerWrappers: wrappers,
	}

	batch(subg)

	return subg // to do more with this routegroup if any
}
