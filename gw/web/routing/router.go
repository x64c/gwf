package routing

import (
	"net/http"

	"github.com/x64c/gwf/gw/web"
)

type Router interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	Handle(pattern string, handler http.Handler, handlerWrappers ...web.HandlerWrapper)
	HandleFunc(pattern string, handleFunc func(http.ResponseWriter, *http.Request), handlerWrappers ...web.HandlerWrapper)
	Group(prefix string, batch func(*RouteGroup), handlerWrappers ...web.HandlerWrapper) *RouteGroup
}
