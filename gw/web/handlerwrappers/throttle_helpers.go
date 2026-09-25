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

// checkBucketGroups is the wrap-time group validation shared by
// ThrottleFixedGroup and ThrottleDynamicGroup, on the NODE plane: routes are
// built before StartServices, so the handle refuses Get() here — but the
// service exists and its groups are already set (boot wiring precedes route
// building). A mistyped group id is a named boot failure instead of a route
// that 429s forever with nothing ever saying why. An app with no throttle
// service at all keeps the absent behavior: wrap proceeds, the absent handle
// answers 503 per request.
func checkBucketGroups(throttleHandle framework.ServiceHandle[throttle.Limiter], groupIDs []string) error {
	if limiter, ok := throttleHandle.Node().Service().(throttle.Limiter); ok {
		for _, id := range groupIDs {
			if !limiter.HasGroup(id) {
				return fmt.Errorf("Throttle: unknown bucket group %q — set it with SetBucketGroup before building routes", id)
			}
		}
	}
	return nil
}

// serveThrottled is the per-request path shared by ThrottleFixedGroup and
// ThrottleDynamicGroup once the bucket group is known: reach the limiter,
// extract the key, ask for a verdict, and either refuse or call inner.
func serveThrottled(w http.ResponseWriter, r *http.Request, inner http.Handler, throttleHandle framework.ServiceHandle[throttle.Limiter], groupID string, keyProvider ThrottleKeyProvider) {
	limiter, ok := throttleHandle.Get()
	if !ok {
		responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.ServiceUnavailable.WithDetail("throttle"))
		return
	}
	key, ok := keyProvider(r)
	if !ok {
		responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited.WithDetail("throttle key extraction failed"))
		return
	}
	allowed, err := limiter.TryAdmit(r.Context(), groupID, key, time.Now())
	if err != nil {
		responses.WriteErrorJSON(w, http.StatusServiceUnavailable, errs.ServiceUnavailable.WithDetail("throttle: no verdict"))
		return
	}
	if !allowed {
		responses.WriteErrorJSON(w, http.StatusTooManyRequests, errs.RateLimited)
		return
	}
	inner.ServeHTTP(w, r)
}
