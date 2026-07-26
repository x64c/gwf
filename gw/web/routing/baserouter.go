package routing

import (
	"net/http"

	"github.com/x64c/gwf/gw/web"
)

type BaseRouter struct {
	*http.ServeMux
}

var _ Router = (*BaseRouter)(nil)

func (r *BaseRouter) Handle(pattern string, handler http.Handler, handlerWrappers ...web.HandlerWrapper) {
	wrappedHandler := handler
	for i := len(handlerWrappers) - 1; i >= 0; i-- {
		wrappedHandler = handlerWrappers[i].Wrap(wrappedHandler)
	}
	r.ServeMux.Handle(pattern, wrappedHandler)
}

func (r *BaseRouter) HandleFunc(pattern string, handleFunc func(http.ResponseWriter, *http.Request), handlerWrappers ...web.HandlerWrapper) {
	r.Handle(pattern, http.HandlerFunc(handleFunc), handlerWrappers...)
}

func (r *BaseRouter) Group(prefix string, batch func(*RouteGroup), handlerWrappers ...web.HandlerWrapper) *RouteGroup {
	g := &RouteGroup{
		Router:          r,
		Prefix:          prefix,
		HandlerWrappers: handlerWrappers,
	}

	batch(g)

	return g
}
