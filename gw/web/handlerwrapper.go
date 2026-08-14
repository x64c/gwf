package web

import "net/http"

// HandlerWrapper has Wrap method which acts as a middleware by wrapping an http.Handler
// prepending and appending some additinonal logic wrapping the handler's ServeHTTP(w,r)
// and then returns a new http.Handler which can wrap another or can be wrapped by another
//
// Wrap runs at BOOT, once per route, while the route tree is built — not per
// request. Its error reports that the wrapper could not be built: a required
// component was never prepared, a field it was given is unusable. On failure
// return (nil, err), as any constructor does; the router stops there, so the
// wrappers outside this one never run against half-built state.
//
// The handler it returns is what serves requests, so the chain composes as
// before: http.Handler in, http.Handler out, to any depth.
type HandlerWrapper interface {
	Wrap(http.Handler) (http.Handler, error)
}
