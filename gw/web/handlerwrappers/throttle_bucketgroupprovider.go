package handlerwrappers

import "net/http"

// ThrottleBucketGroupProvider picks, for ThrottleDynamicGroup, the bucket group
// — the rate — a request is counted in. BucketGroupIDs lists every group
// BucketGroupID may return, and ThrottleDynamicGroup checks each against the
// limiter at wrap time, so a group the code names but the configuration lacks
// fails the boot.
//
// BucketGroupID picks from the request's own data only, so every process of
// the app picks the same group for the same request. It returns (_, false)
// when it cannot pick, and ThrottleDynamicGroup then blocks the request.
type ThrottleBucketGroupProvider interface {
	BucketGroupIDs() []string
	BucketGroupID(r *http.Request) (string, bool)
}
