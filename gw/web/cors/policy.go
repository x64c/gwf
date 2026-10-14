package cors

import (
	"fmt"
	"net/http"
)

// Policy is a CORS policy for the routes it is applied to, one route or many groups: the deployment half (Conf, from
// Confs by its access control key) and the code half (the methods and request headers the routes' pages use).
//
// Applied as a handler wrapper, it lets its origins read the responses of the routes it wraps. Put it first in their
// wrapper list, before authentication, so every answer carries the header — a refusal, an error, a stream. From an
// allowed origin, a response gets Access-Control-Allow-Origin: <that origin> and Vary: Origin; under ["*"], every
// response gets Access-Control-Allow-Origin: *. Any other request passes through with only Vary: Origin, and the
// browser keeps the response from the page. It sets headers before calling the next handler and never wraps the
// ResponseWriter, so a stream still flushes. PreflightHandler answers the same routes' preflights.
type Policy struct {
	Conf                  // REQUIRED. From Confs, by its access control key.
	AllowMethods []string // The methods beyond GET/HEAD/POST a page may use (Access-Control-Allow-Methods); required to build a preflight handler.
	AllowHeaders []string // The request headers beyond the CORS-safelisted ones a page may send (Access-Control-Allow-Headers) — a permission, not a requirement.
}

// Wrap refuses at boot a Conf that Conf.Validate refuses; it uses only the Conf.
func (p Policy) Wrap(inner http.Handler) (http.Handler, error) {
	if err := p.Conf.Validate(); err != nil {
		return nil, fmt.Errorf("cors: %w", err)
	}
	if p.AllowOrigins[0] == "*" { // Validate: "*" stands alone
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			inner.ServeHTTP(w, r)
		}), nil
	}
	origins := make(map[string]struct{}, len(p.AllowOrigins))
	for _, o := range p.AllowOrigins {
		origins[o] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", "Origin") // the answer differs by Origin, even when it carries no header
		if origin := r.Header.Get("Origin"); origin != "" {
			if _, ok := origins[origin]; ok {
				h.Set("Access-Control-Allow-Origin", origin)
			}
		}
		inner.ServeHTTP(w, r)
	}), nil
}
