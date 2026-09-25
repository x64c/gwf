package handlerwrappers

import (
	"net/http"

	"github.com/x64c/gwf/gw/framework"
)

// ThrottleKeyProvider extracts the throttle bucket key from the request.
// Returns (key, true) on success; (_, false) signals failure to extract,
// in which case the throttle blocks the request.
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
