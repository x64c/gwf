package handlerwrappers

import (
	"net/http"
	"time"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// ThrottleKeyProvider extracts the throttle bucket key from the request.
// Returns (key, true) on success; (_, false) signals failure to extract,
// in which case Throttle blocks the request.
type ThrottleKeyProvider func(*http.Request) (string, bool)

// Throttle limits requests by a caller-defined string key. The KeyProvider
// closure decides where the key comes from (session UID, session ID, IP,
// composite, etc.) — Throttle itself is key-source-agnostic.
type Throttle struct {
	AppProvider   framework.AppProviderFunc
	BucketGroupID string
	KeyProvider   ThrottleKeyProvider
}

func (m *Throttle) Wrap(inner http.Handler) http.Handler {
	appCore := m.AppProvider().AppCore()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := m.KeyProvider(r)
		if !ok {
			responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited.WithDetail("throttle key extraction failed"))
			return
		}
		if !appCore.ThrottleService.Allow(m.BucketGroupID, key, time.Now()) {
			responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited)
			return
		}
		inner.ServeHTTP(w, r)
	})
}
