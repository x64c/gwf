package handlerwrappers

import (
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/requests"
	"github.com/x64c/gwf/gw/web/responses"
)

type ThrottleIP struct {
	AppProvider   framework.AppProviderFunc
	BucketGroupID string
}

func (m *ThrottleIP) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Requested IP
		ip := requests.GetClientIP(r)
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
