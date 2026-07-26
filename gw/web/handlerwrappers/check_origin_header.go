package handlerwrappers

import (
	"log"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/requests"
	"github.com/x64c/gwf/gw/web/responses"
)

// CheckOriginHeader checks the request's Origin header against an allowlist and
// rejects non-matching requests with 403. It is a CSRF defense layer: the
// Origin header is browser-set and not forgeable by cross-origin JavaScript
// (it's in the Fetch spec's forbidden-header list), so for browser-driven
// requests it's a reliable "which origin initiated this" signal.
//
// The name is literal: it checks the Origin HEADER specifically — not an
// abstract "same-origin" guarantee. There is no Referer fallback (see below).
//
// Allowlist: the app's canonical origin is read from appCore.Host (set in
// .core.json). ExtraOrigins adds additional allowed origins (e.g. legacy
// hostnames during a migration).
//
// Method scope:
//   - Default (Strict=false): safe methods (GET, HEAD, OPTIONS) pass through
//     unchecked. Safe methods shouldn't have side effects (so no CSRF risk),
//     and GETs are meant to be reachable cross-origin (links, bookmarks,
//     search engines) — enforcing origin on them would break normal web
//     navigation. Cross-origin READ of a GET response is already blocked by
//     the browser's Same-Origin Policy, so origin-checking GETs adds nothing.
//   - Strict (Strict=true): every method is checked, including safe ones.
//     Purpose: deliberately apply this middleware to endpoints you have
//     explicitly determined ALWAYS receive an Origin header — i.e. AJAX/
//     programmatic-only endpoints (fetch/XHR from your own SPA, JSON APIs)
//     that are never reached by direct browser navigation. For those, even
//     GETs carry Origin (the browser sets it on fetch/XHR), so enforcing on
//     all methods is both safe and tighter.
//     The dev makes this "always has Origin" determination per endpoint; the
//     middleware does not guess. DANGER: enabling Strict on a route reachable
//     by direct URL navigation (typed URL, bookmark, external link) blocks
//     legitimate visits — direct navigation sends no Origin, so it 403s.
//
// Single signal — Origin only:
// There is intentionally NO Referer fallback. Origin and Referer can carry
// different information (Origin goes "null" on cross-origin redirects and for
// opaque origins; Referer is subject to Referrer-Policy stripping). When both
// are present and non-null from a real browser, they agree — so Referer adds
// nothing. When they disagree, it's a forged/non-browser client or the browser
// deliberately signaling "untrusted" via Origin: null — consulting Referer
// there would override the browser's signal. So a missing or mismatched Origin
// fails closed (rejected); we do not look at Referer to rescue it.
//
// What it does NOT defend: non-browser clients (curl, HTTP libraries, attack
// tools) can forge the Origin header — this middleware only constrains
// browser-driven requests. That's exactly the CSRF threat model (the browser
// is the unwitting attack vehicle). Pair with CSRFToken for defense-in-depth:
// CSRFToken proves the request body was rendered by our origin's DOM,
// CheckOriginHeader proves the request was initiated from our origin.
//
// Scope it to cookie-session route groups (where CSRF applies). Bearer-token
// JSON APIs don't need it — tokens aren't auto-attached cross-origin.
//
// Boot-time: log.Fatal if appCore.Host is empty — devs must set it in
// .core.json before this middleware is wired.
type CheckOriginHeader struct {
	AppProvider  framework.AppProviderFunc
	ExtraOrigins []string
	Strict       bool // true = check all methods incl. GET/HEAD/OPTIONS. Default false = skip safe methods.
}

func (m *CheckOriginHeader) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	if appCore.Host == "" {
		log.Fatal("[ERROR] CheckOriginHeader: appCore.Host is empty — set \"host\" in .core.json")
	}
	set := map[string]struct{}{appCore.Host: {}}
	for _, o := range m.ExtraOrigins {
		if o == "" {
			log.Fatal("[ERROR] CheckOriginHeader: \"\" not allowed in ExtraOrigins")
		}
		set[o] = struct{}{}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.Strict && requests.IsSafeMethod(r) {
			inner.ServeHTTP(w, r)
			return
		}
		origin := r.Header.Get("Origin")
		// Missing Origin always fails closed — independent of allowlist contents.
		if origin == "" {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.OriginNotAllowed.WithDetail("missing Origin header"))
			return
		}
		// Origin only — no Referer fallback.
		if _, ok := set[origin]; !ok {
			responses.WriteErrorJSON(w, http.StatusForbidden, errs.OriginNotAllowed.WithDetail("origin: "+origin))
			return
		}
		inner.ServeHTTP(w, r)
	})
}
