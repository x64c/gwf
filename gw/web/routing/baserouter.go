package routing

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/web"
)

// BaseRouter is the reference implementation of Router: routing through the
// embedded http.ServeMux, with wrapper composition on Handle, HandleFunc and
// Group. It recovers no panic itself; a recovery placed in the chain catches
// one.
type BaseRouter struct {
	*http.ServeMux
}

var _ Router = (*BaseRouter)(nil)

func (r *BaseRouter) Handle(pattern string, handler http.Handler, handlerWrappers ...web.HandlerWrapper) {
	// Built inner-first; a failure stops the chain there. See RouteGroup.Handle
	// for why this frame's disposal of the error is still its existing behavior.
	wrappedHandler := handler
	for i := len(handlerWrappers) - 1; i >= 0; i-- {
		wrapped, err := handlerWrappers[i].Wrap(wrappedHandler)
		if err != nil {
			log.Fatalf("[ERROR] route %q: %v", pattern, err)
		}
		wrappedHandler = wrapped
	}
	r.ServeMux.Handle(pattern, wrappedHandler)
}

func (r *BaseRouter) HandleFunc(pattern string, handleFunc func(http.ResponseWriter, *http.Request), handlerWrappers ...web.HandlerWrapper) {
	r.Handle(pattern, http.HandlerFunc(handleFunc), handlerWrappers...)
}

func (r *BaseRouter) Group(prefix string, batch func(*RouteGroup), handlerWrappers ...web.HandlerWrapper) *RouteGroup {
	g := &RouteGroup{
		Router: r,
		Prefix: prefix,
		// The group owns its wrapper list (see RouteGroup.Group) — a caller
		// spreading a shared slice must not alias it into the group.
		HandlerWrappers: append(make([]web.HandlerWrapper, 0, len(handlerWrappers)), handlerWrappers...),
	}

	batch(g)

	return g
}
