package handlerwrappers

import (
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

type ThrottleIP struct {
	AppProvider   framework.AppProviderFunc
	BucketGroupID string
}

func (m *ThrottleIP) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Requested IP — resolved per the deployment's trusted proxies. Keying a
		// limiter by address is only as meaningful as that declaration: with none,
		// every request behind a proxy shares one bucket.
		ip := appCore.ClientIPResolver.ClientIP(r)
		// Check Throttle Bucket
		if !appCore.ThrottleService.Allow(m.BucketGroupID, ip, time.Now()) {
			responses.WriteSimpleErrorJSON(w, http.StatusTooManyRequests, "access rate limited - ip "+ip)
			return
		}

		// Inner
		inner.ServeHTTP(w, r)

		// Post-action
	})
}
