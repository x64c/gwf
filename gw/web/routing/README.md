# Routing System

## - Handler Wrapper Order

The variadic `handlerWrappers ...web.HandlerWrapper` accepted by `Handle`,
`HandleFunc`, and `Group` are applied **outer-to-inner from left to right**.
The earlier argument wraps the result of the later arguments.

```go
router.Handle(pattern, handler, w1, w2)
```

builds:

```
w1( w2( handler ) )
```

On each request, `w1`'s `Wrap` body runs first, then calls into `w2`'s `Wrap`
body, then into `handler`. Response unwinding goes in reverse order.

When mixed with `Group` middleware, group middlewares are outer to per-route
middlewares:

```go
router.Group(prefix, batch, gw1, gw2)
//   inside the group: g.Handle(p, h, rw1, rw2)
```

builds:

```
gw1( gw2( rw1( rw2( handler ) ) ) )
```

i.e. group wrappers run before per-route wrappers on each request.

**Practical implication:** put pre-checks that should reject early (e.g. body
limit, auth, session validation) **earlier** in the vararg list (and at the
group level if they apply group-wide). Put inner concerns (response
decoration, body parsing helpers) later.

## - Just Using Bundled BaseRouter and RouteGroup

```
type Router = routing.BaseRouter
type RouteGroup = routing.RouteGroup
```

## - Base Middleware to Apply Entire Requests
```
server := &http.Server{
    Addr:    env.Listen,
    Handler: HttpHandlerWrapper(router),
}
```
where `HttpHandlerWrapper` is a `func(http.Handler) http.Handler`

## - Overriding Methods by Embedding
You can write your own router.

e.g.
```
type Router struct {
	routing.BaseRouter
}

func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
    ...
    ctx := foo.WithFooConf(r.Context(), bar)
    router.ServeMux.ServeHTTP(w, r.WithContext(ctx))
    ...
}

// still, your RouteGroup can be an alias with type instantiation:
type RouteGroup = routing.RouteGroup
```
