package cors

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/responses"
)

// PreflightHandler builds the handler that answers the preflight requests for the routes policy is applied to.
// Register it on an OPTIONS route covering those routes, outside every authentication wrapper: a preflight carries no
// credentials.
//
// A preflight — OPTIONS with Origin and Access-Control-Request-Method — from an origin policy allows, asking for a
// method and request headers policy allows, gets 204 with Access-Control-Allow-Origin, -Allow-Methods, -Allow-Headers
// and -Max-Age. Any other origin, method or header gets 403; an OPTIONS request that is not a preflight gets 400.
// Refused when building: a Conf that Conf.Validate refuses, no AllowMethods, a method or header that is not a token.
func PreflightHandler(policy Policy) (http.Handler, error) {
	if err := policy.Conf.Validate(); err != nil {
		return nil, fmt.Errorf("cors: %w", err)
	}
	anyOrigin := policy.AllowOrigins[0] == "*" // Validate: "*" stands alone
	origins := make(map[string]struct{}, len(policy.AllowOrigins))
	for _, o := range policy.AllowOrigins {
		origins[o] = struct{}{}
	}
	if len(policy.AllowMethods) == 0 {
		return nil, fmt.Errorf("cors: a preflight needs AllowMethods")
	}
	// "*" in these lists is a browser-side wildcard (never covering Authorization); a list here means exactly what it
	// names, so it is refused.
	methods := make(map[string]struct{}, len(policy.AllowMethods))
	for _, m := range policy.AllowMethods {
		if m == "*" || !isToken(m) {
			return nil, fmt.Errorf("cors: %q is not a method", m)
		}
		methods[m] = struct{}{}
	}
	headers := make(map[string]struct{}, len(policy.AllowHeaders))
	for _, h := range policy.AllowHeaders {
		if h == "*" || !isToken(h) {
			return nil, fmt.Errorf("cors: %q is not a header name", h)
		}
		headers[strings.ToLower(h)] = struct{}{}
	}

	allowMethods := strings.Join(policy.AllowMethods, ", ")
	allowHeaders := strings.Join(policy.AllowHeaders, ", ")
	maxAge := strconv.Itoa(policy.AccessControlMaxAgeSecs)
	vary := "Access-Control-Request-Method, Access-Control-Request-Headers"
	if !anyOrigin {
		vary = "Origin, " + vary
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Add("Vary", vary)
		origin := r.Header.Get("Origin")
		method := r.Header.Get("Access-Control-Request-Method")
		if r.Method != http.MethodOptions || origin == "" || method == "" {
			responses.WriteSimpleErrorJSON(w, http.StatusBadRequest, "cors: not a preflight request (OPTIONS with Origin and Access-Control-Request-Method)")
			return
		}
		if !anyOrigin {
			if _, ok := origins[origin]; !ok {
				responses.WriteErrorJSON(w, http.StatusForbidden, errs.OriginNotAllowed.WithDetail("origin: "+origin))
				return
			}
		}
		// A CORS-safelisted method needs no listing: a browser allows it whatever Allow-Methods says.
		if _, ok := methods[method]; !ok && method != http.MethodGet && method != http.MethodHead && method != http.MethodPost {
			responses.WriteSimpleErrorJSON(w, http.StatusForbidden, "cors: method not allowed: "+method)
			return
		}
		for name := range strings.SplitSeq(r.Header.Get("Access-Control-Request-Headers"), ",") {
			name = strings.ToLower(strings.TrimSpace(name))
			if name == "" {
				continue
			}
			if _, ok := headers[name]; !ok {
				responses.WriteSimpleErrorJSON(w, http.StatusForbidden, "cors: request header not allowed: "+name)
				return
			}
		}
		if anyOrigin {
			h.Set("Access-Control-Allow-Origin", "*")
		} else {
			h.Set("Access-Control-Allow-Origin", origin)
		}
		h.Set("Access-Control-Allow-Methods", allowMethods)
		if allowHeaders != "" {
			h.Set("Access-Control-Allow-Headers", allowHeaders)
		}
		h.Set("Access-Control-Max-Age", maxAge)
		w.WriteHeader(http.StatusNoContent)
	}), nil
}

// isToken reports whether s is an HTTP token (RFC 9110 §5.6.2) — what a method or a header name must be.
func isToken(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}
