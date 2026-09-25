package handlerwrappers

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/framework"
	"github.com/x64c/gwf/gw/web/responses"
)

// ThrottleDynamicGroup is ThrottleFixedGroup with the bucket group — the rate —
// picked per request by BucketGroupProvider, so one route can count different
// callers at different rates. Every group the provider lists is checked at wrap
// time; per request, a provider that cannot pick blocks the request with 429,
// and a pick it did not list answers 500 rather than being counted where no
// one declared it would be. Past the pick it behaves as ThrottleFixedGroup.
type ThrottleDynamicGroup struct {
	AppProvider         framework.AppProviderFunc
	BucketGroupProvider ThrottleBucketGroupProvider
	KeyProvider         ThrottleKeyProvider
}

func (m *ThrottleDynamicGroup) Wrap(inner http.Handler) (http.Handler, error) {
	if m.BucketGroupProvider == nil {
		return nil, errors.New("ThrottleDynamicGroup: BucketGroupProvider required")
	}
	groupIDs := m.BucketGroupProvider.BucketGroupIDs()
	if len(groupIDs) == 0 {
		return nil, errors.New("ThrottleDynamicGroup: BucketGroupProvider lists no bucket group")
	}
	listed := make(map[string]struct{}, len(groupIDs))
	for _, id := range groupIDs {
		listed[id] = struct{}{}
	}
	throttleHandle := m.AppProvider().AppCore().ThrottleHandle()
	if err := checkBucketGroups(throttleHandle, groupIDs); err != nil {
		return nil, err
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := m.BucketGroupProvider.BucketGroupID(r)
		if !ok {
			responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited.WithDetail("throttle bucket group selection failed"))
			return
		}
		if _, ok := listed[groupID]; !ok {
			responses.WriteErrorJSON(w, http.StatusInternalServerError, errs.InternalError.WithDetail(fmt.Sprintf("throttle bucket group %q not listed by its provider", groupID)))
			return
		}
		serveThrottled(w, r, inner, throttleHandle, groupID, m.KeyProvider)
	}), nil
}
