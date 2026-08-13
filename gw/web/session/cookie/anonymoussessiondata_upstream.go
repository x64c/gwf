package cookie

import (
	"context"
	"net/http"
	"sync"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

// The upstream request ladder lives once, in fwupstream, written against its
// TokenRow constraint. The first five methods below are this shape's half of
// that constraint — its identity, computed from what it already holds. The
// rest is the published method surface, delegating to the shared ladder.

// UpstreamHub reports which Hub owns this session's upstream tokens.
func (sd *AnonymousSessionData) UpstreamHub() *fwupstream.Hub { return sd.Mgr.FWUpstream }

// UpstreamRowKey returns the KVDB row key this session's upstream tokens are
// stored on — the same row the session itself lives in.
func (sd *AnonymousSessionData) UpstreamRowKey() string { return sd.Mgr.AnonymousSessionRowKey(sd.ID) }

// UpstreamAccessSlots returns this request's cache of upstream access-token
// reads.
func (sd *AnonymousSessionData) UpstreamAccessSlots() *fwupstream.TknSlots { return &sd.upATknSlots }

// UpstreamRefreshSlots returns this request's cache of upstream refresh-token
// reads.
func (sd *AnonymousSessionData) UpstreamRefreshSlots() *fwupstream.TknSlots { return &sd.upRTknSlots }

// UpstreamRefreshLocker returns the per-row mutex guarding this session's
// upstream refresh, from the store shared with session.Service. Keyed by the
// row key — the same value UpstreamRowKey computes — so every request on this
// session takes the same lock.
func (sd *AnonymousSessionData) UpstreamRefreshLocker() sync.Locker {
	return sd.Mgr.SessionLocks.Acquire(sd.Mgr.AnonymousSessionRowKey(sd.ID))
}

// UpstreamRefreshExtras returns the extra body fields for the refresh request,
// from the sideloader registered for this shape via
// SetAnonymousRefreshSideloader.
func (sd *AnonymousSessionData) UpstreamRefreshExtras(ctx context.Context, fwClient *fwupstream.Client) (map[string]any, *errs.Error) {
	refreshSideloader, ok := GetAnonymousRefreshSideloader(fwClient)
	if !ok {
		return nil, errs.UpstreamRefreshSideloaderNotSet.WithDetail("Client " + fwClient.ID + " has no AnonymousRefreshSideloader")
	}
	// sd is this method's receiver — forwarded so the closure body never has to fetch session data from ctx.
	return refreshSideloader(ctx, sd), nil
}

// UpstreamAccessToken returns the upstream access token for clientID, fetching
// it from KVDB on first call and caching the result for the rest of the
// request. See fwupstream.RowAccessToken.
func (sd *AnonymousSessionData) UpstreamAccessToken(ctx context.Context, clientID string) (string, *errs.Error) {
	return fwupstream.RowAccessToken(ctx, sd, clientID)
}

// UpstreamRefreshToken returns the upstream refresh token for clientID,
// fetching it from KVDB on first call and caching the result for the rest of
// the request. See fwupstream.RowRefreshToken.
func (sd *AnonymousSessionData) UpstreamRefreshToken(ctx context.Context, clientID string) (string, *errs.Error) {
	return fwupstream.RowRefreshToken(ctx, sd, clientID)
}

// UpstreamRequestWithBearer is the foundation method for sending a request
// with an access token from the session. Returns the raw *http.Response
// (caller closes body) on success. See fwupstream.RowRequestWithBearer.
func (sd *AnonymousSessionData) UpstreamRequestWithBearer(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RowRequestWithBearer(ctx, sd, fwClient, method, endpoint, payload)
}

// UpstreamRequestWithBearerRetriable sends the request and, on a
// refresh-worthy auth failure (token missing locally, or upstream rejected
// it), refreshes the token pair, updates the session row + cached slots, and
// retries once. Refresh-request body extras come from the sideloader
// registered via SetAnonymousRefreshSideloader; without one, returns
// UpstreamRefreshSideloaderNotSet. See fwupstream.RowRequestWithBearerRetriable.
func (sd *AnonymousSessionData) UpstreamRequestWithBearerRetriable(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RowRequestWithBearerRetriable(ctx, sd, fwClient, method, endpoint, payload)
}

// UpstreamRequestJSON forces Accept: application/json and delegates to the
// retriable path. See fwupstream.RequestJSON.
func (sd *AnonymousSessionData) UpstreamRequestJSON(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RequestJSON(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}

// UpstreamFetchJSON calls UpstreamRequestJSON and unmarshals the response body
// into target. See fwupstream.FetchJSON.
func (sd *AnonymousSessionData) UpstreamFetchJSON(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload, target any) (http.Header, int, *errs.Error) {
	return fwupstream.FetchJSON(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload, target)
}

// UpstreamRequestPDF forces Accept: application/pdf and delegates to the
// retriable path. See fwupstream.RequestPDF.
func (sd *AnonymousSessionData) UpstreamRequestPDF(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RequestPDF(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}

// UpstreamFetchPDFBytes calls UpstreamRequestPDF and reads the response body
// into []byte. See fwupstream.FetchPDFBytes.
func (sd *AnonymousSessionData) UpstreamFetchPDFBytes(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) ([]byte, http.Header, int, *errs.Error) {
	return fwupstream.FetchPDFBytes(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}
