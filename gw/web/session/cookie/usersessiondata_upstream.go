package cookie

import (
	"context"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

// The upstream request ladder lives once, in fwupstream, written against its
// TokenRow constraint. The first five methods below are this shape's half of
// that constraint — its identity, computed from what it already holds. The
// rest is the published method surface, delegating to the shared ladder.

// UpstreamHub reports which Hub owns this session's upstream tokens.
func (sd *UserSessionData[UID]) UpstreamHub() *fwupstream.Hub { return sd.Mgr.FWUpstream }

// UpstreamRowKey returns the KVDB row key this session's upstream tokens are
// stored on — the same row the session itself lives in.
func (sd *UserSessionData[UID]) UpstreamRowKey() string { return sd.Mgr.UserSessionRowKey(sd.ID) }

// UpstreamAccessSlots returns this request's cache of upstream access-token
// reads.
func (sd *UserSessionData[UID]) UpstreamAccessSlots() *fwupstream.TknSlots { return &sd.upATknSlots }

// UpstreamRefreshSlots returns this request's cache of upstream refresh-token
// reads.
func (sd *UserSessionData[UID]) UpstreamRefreshSlots() *fwupstream.TknSlots { return &sd.upRTknSlots }

// HoldUpstreamRefresh runs fn holding this session's upstream refresh, on
// the manager shared with session.Service. Named by the row key — the same
// value UpstreamRowKey computes — so every request on this session holds the
// same name. Refused at once while another request holds it.
func (sd *UserSessionData[UID]) HoldUpstreamRefresh(ctx context.Context, fn func(ctx context.Context) error) error {
	return sd.Mgr.lockingManager.AcquireDoRelease(ctx, sd.Mgr.UserSessionRowKey(sd.ID), fn)
}

// UpstreamRefreshExtras returns the extra body fields for the refresh request,
// from the sideloader registered for this shape via SetUserRefreshSideloader.
func (sd *UserSessionData[UID]) UpstreamRefreshExtras(ctx context.Context, fwClient *fwupstream.Client) (map[string]any, *errs.Error) {
	refreshSideloader, ok := GetUserRefreshSideloader[UID](fwClient)
	if !ok {
		return nil, errs.UpstreamRefreshSideloaderNotSet.WithDetail("Client " + fwClient.ID + " has no UserRefreshSideloader")
	}
	// sd is this method's receiver — forwarded so the closure body never has to fetch session data from ctx.
	return refreshSideloader(ctx, sd), nil
}

// UpstreamAccessToken returns the upstream access token for clientID, fetching
// it from KVDB on first call and caching the result for the rest of the
// request. See fwupstream.RowAccessToken.
func (sd *UserSessionData[UID]) UpstreamAccessToken(ctx context.Context, clientID string) (string, *errs.Error) {
	return fwupstream.RowAccessToken(ctx, sd, clientID)
}

// UpstreamRefreshToken returns the upstream refresh token for clientID,
// fetching it from KVDB on first call and caching the result for the rest of
// the request. See fwupstream.RowRefreshToken.
func (sd *UserSessionData[UID]) UpstreamRefreshToken(ctx context.Context, clientID string) (string, *errs.Error) {
	return fwupstream.RowRefreshToken(ctx, sd, clientID)
}

// UpstreamRequestWithBearer is the foundation method for sending a request
// with an access token from the session. Returns the raw *http.Response
// (caller closes body) on success. See fwupstream.RowRequestWithBearer.
func (sd *UserSessionData[UID]) UpstreamRequestWithBearer(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RowRequestWithBearer(ctx, sd, fwClient, method, endpoint, payload)
}

// UpstreamRequestWithBearerRetriable sends the request and, on a
// refresh-worthy auth failure (token missing locally, or upstream rejected
// it), obtains a usable token pair — refreshing it, or adopting one another
// request refreshed meanwhile — and retries; while another request's
// refresh is in flight it follows fwupstream.RetriableOnceInASec.
// Refresh-request body extras come from the sideloader registered via
// SetUserRefreshSideloader; without one, returns
// UpstreamRefreshSideloaderNotSet. See fwupstream.RowRequestWithBearerRetriable.
func (sd *UserSessionData[UID]) UpstreamRequestWithBearerRetriable(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RowRequestWithBearerRetriable(ctx, sd, fwClient, method, endpoint, payload, fwupstream.RetriableOnceInASec)
}

// UpstreamRequestWithExchangeRetriable sends the request through the
// session's exchanged upstream bearer — the user token the upstream issued
// for this session's user against the downstream's machine assertion — cached on
// the row or exchanged on demand (fwClient.Signer required); on a 401 it
// exchanges once more and retries once. The upstream's status is reported
// as-is. See fwupstream.RowRequestWithExchangeRetriable.
func (sd *UserSessionData[UID]) UpstreamRequestWithExchangeRetriable(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RowRequestWithExchangeRetriable(ctx, sd, fwClient, sd.UID, method, endpoint, payload)
}

// UpstreamForgetExchanged drops the session's exchanged upstream bearer,
// revoking it at the upstream first when revoke is set. See
// fwupstream.RowForgetExchanged.
func (sd *UserSessionData[UID]) UpstreamForgetExchanged(ctx context.Context, fwClient *fwupstream.Client, revoke bool) *errs.Error {
	return fwupstream.RowForgetExchanged(ctx, sd, fwClient, revoke)
}

// UpstreamRequestJSON forces Accept: application/json and delegates to the
// retriable path. See fwupstream.RequestJSON.
func (sd *UserSessionData[UID]) UpstreamRequestJSON(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RequestJSON(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}

// UpstreamFetchJSON calls UpstreamRequestJSON and unmarshals the response body
// into target. See fwupstream.FetchJSON.
func (sd *UserSessionData[UID]) UpstreamFetchJSON(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload, target any) (http.Header, int, *errs.Error) {
	return fwupstream.FetchJSON(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload, target)
}

// UpstreamRequestPDF forces Accept: application/pdf and delegates to the
// retriable path. See fwupstream.RequestPDF.
func (sd *UserSessionData[UID]) UpstreamRequestPDF(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) (*http.Response, int, *errs.Error) {
	return fwupstream.RequestPDF(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}

// UpstreamFetchPDFBytes calls UpstreamRequestPDF and reads the response body
// into []byte. See fwupstream.FetchPDFBytes.
func (sd *UserSessionData[UID]) UpstreamFetchPDFBytes(ctx context.Context, fwClient *fwupstream.Client, method, endpoint string, payload *fwupstream.RequestPayload) ([]byte, http.Header, int, *errs.Error) {
	return fwupstream.FetchPDFBytes(ctx, sd.UpstreamRequestWithBearerRetriable, fwClient, method, endpoint, payload)
}
