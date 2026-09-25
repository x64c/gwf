package handlerwrappers

import (
	"net/http"

	"github.com/x64c/gwf/gw/framework"
)

// ThrottleFixedGroup limits requests by a caller-defined string key within one
// bucket group, BucketGroupID. The KeyProvider closure decides where the key
// comes from (session UID, session ID, IP, composite, etc.) — the throttle
// itself is key-source-agnostic.
//
// The limiter is reached through its framework handle. One that cannot answer
// must not wave traffic through, so an un-admitted service — stopped by an
// operator, mid-teardown, or never wired — fails CLOSED: 503, not a rate
// verdict nobody computed. A limiter that is admitted but returns an error
// fails closed the same way, for the same reason: it computed nothing either.
// Only a limiter whose counters live outside this process can reach that
// second branch.
type ThrottleFixedGroup struct {
	AppProvider   framework.AppProviderFunc
	BucketGroupID string
	KeyProvider   ThrottleKeyProvider
}

func (m *ThrottleFixedGroup) Wrap(inner http.Handler) (http.Handler, error) {
	throttleHandle := m.AppProvider().AppCore().ThrottleHandle()
	if err := checkBucketGroups(throttleHandle, []string{m.BucketGroupID}); err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveThrottled(w, r, inner, throttleHandle, m.BucketGroupID, m.KeyProvider)
	}), nil
}
