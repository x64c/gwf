package requests

import "net/http"

func HasBody(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		// bodiless
		return false
	default:
		return true
	}
}

// IsSafeMethod reports whether the request method is "safe" per RFC 9110:
// no expected side effects on the server (GET, HEAD, OPTIONS). Used by
// middlewares that may skip state-change defenses on safe methods (e.g.
// CSRF, CheckOriginHeader).
//
// Note: the answer set coincides with !HasBody(r) today, but the semantics
// differ — this predicate is about method side-effect semantics, not body
// presence. They may diverge if HTTP ever evolves a body-carrying safe method.
func IsSafeMethod(r *http.Request) bool {
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}
