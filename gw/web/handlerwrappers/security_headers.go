package handlerwrappers

import "net/http"

// SecurityHeaders sets response security headers. Empty string for any field
// means "don't set that header" (so the edge-layer value, if any, passes
// through unchanged).
//
// Apply on route groups (or single routes) that need stricter policies than
// the edge-layer baseline. Common cases:
//   - JSON-API route groups: typically need only nosniff + HSTS (often
//     already set at the edge; this middleware can refine per-group).
//   - Web-app route groups: usually want a Content-Security-Policy and
//     X-Frame-Options matching the page's needs.
//
// Headers are set BEFORE the inner handler runs, so they apply to all
// responses (success and error paths).
type SecurityHeaders struct {
	ContentTypeOptions    string // e.g. "nosniff"
	StrictTransport       string // HSTS, e.g. "max-age=63072000; includeSubDomains"
	ContentSecurityPolicy string // CSP, e.g. "default-src 'self'; script-src 'self'"
	FrameOptions          string // X-Frame-Options, e.g. "DENY" or "SAMEORIGIN"
	ReferrerPolicy        string // e.g. "strict-origin-when-cross-origin"
	PermissionsPolicy     string // e.g. "camera=(), microphone=(), geolocation=()"
}

func (m *SecurityHeaders) Wrap(inner http.Handler) (http.Handler, error) {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		if m.ContentTypeOptions != "" {
			h.Set("X-Content-Type-Options", m.ContentTypeOptions)
		}
		if m.StrictTransport != "" {
			h.Set("Strict-Transport-Security", m.StrictTransport)
		}
		if m.ContentSecurityPolicy != "" {
			h.Set("Content-Security-Policy", m.ContentSecurityPolicy)
		}
		if m.FrameOptions != "" {
			h.Set("X-Frame-Options", m.FrameOptions)
		}
		if m.ReferrerPolicy != "" {
			h.Set("Referrer-Policy", m.ReferrerPolicy)
		}
		if m.PermissionsPolicy != "" {
			h.Set("Permissions-Policy", m.PermissionsPolicy)
		}
		inner.ServeHTTP(w, r)
	}), nil
}
