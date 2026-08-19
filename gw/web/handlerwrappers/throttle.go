package handlerwrappers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/throttle"
	"github.com/x64c/gwf/gw/web/responses"
)

// ThrottleKeyProvider extracts the throttle bucket key from the request.
// Returns (key, true) on success; (_, false) signals failure to extract,
// in which case Throttle blocks the request.
type ThrottleKeyProvider func(*http.Request) (string, bool)

// IPThrottleKey keys the limiter by the resolved client address, per the
// deployment's trusted proxies (ClientIPResolver). The resolution itself never
// fails — with no trusted proxies the answer is the connection peer — so the
// provider always reports ok.
//
// An address is not a principal: many callers share one (NAT, single-egress
// networks), addresses are reassigned, and IPv6 makes them cheap to rotate. A
// per-address limit is a volumetric control — it bounds traffic from one
// network path, not attempts by one identity. Where the request carries an
// authenticated identity, key the limiter on that identity and use the
// per-address limit only as the outer bound. See throttle.md.
func IPThrottleKey(appProvider framework.AppProviderFunc) ThrottleKeyProvider {
	return func(r *http.Request) (string, bool) {
		return appProvider().AppCore().ClientIPResolver.ClientIP(r), true
	}
}

// Throttle limits requests by a caller-defined string key. The KeyProvider
// closure decides where the key comes from (session UID, session ID, IP,
// composite, etc.) — Throttle itself is key-source-agnostic.
//
// The throttle service is reached through its framework handle. A limiter that
// cannot answer must not wave traffic through, so an un-admitted service —
// stopped by an operator, mid-teardown, or never wired — fails CLOSED: 503,
// not a rate verdict nobody computed.
type Throttle struct {
	AppProvider   framework.AppProviderFunc
	BucketGroupID string
	KeyProvider   ThrottleKeyProvider
}

func (m *Throttle) Wrap(inner http.Handler) (http.Handler, error) {
	throttleHandle := m.AppProvider().AppCore().ThrottleHandle()
	// Wrap-time group validation, on the NODE plane: routes are built before
	// StartServices, so the handle refuses Get() here — but the service exists
	// and its groups are already set (boot wiring precedes route building). A
	// mistyped group id is a named boot failure instead of a route that 429s
	// forever with nothing ever saying why. An app with no throttle service at
	// all keeps the absent behavior: wrap proceeds, the absent handle answers
	// 503 per request.
	if svc, ok := throttleHandle.Node().Service().(*throttle.Service); ok {
		if !svc.HasGroup(m.BucketGroupID) {
			return nil, fmt.Errorf("Throttle: unknown bucket group %q — set it with SetBucketGroup before building routes", m.BucketGroupID)
		}
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		throttleSvc, ok := throttleHandle.Get()
		if !ok {
			responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.ServiceUnavailable.WithDetail("throttle"))
			return
		}
		key, ok := m.KeyProvider(r)
		if !ok {
			responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited.WithDetail("throttle key extraction failed"))
			return
		}
		if !throttleSvc.Allow(m.BucketGroupID, key, time.Now()) {
			responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited)
			return
		}
		inner.ServeHTTP(w, r)
	}), nil
}
