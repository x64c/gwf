package cookie

import (
	"context"
	"net/http"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/web/fwupstream"
)

// UpstreamAccessToken returns the upstream access token for clientID, fetching
// it from KVDB on first call and caching the result for the rest of the
// request. Concurrent calls for the same clientID share a single fetch.
func (sd *AnonymousSessionData) UpstreamAccessToken(ctx context.Context, clientID string) (string, *errs.Error) {
	slot, _ := sd.upATknSlots.LoadOrStore(clientID, &fwupstream.TknSlot{})
	s := slot.(*fwupstream.TknSlot)
	s.Once.Do(func() {
		s.Val, s.Err = sd.Mgr.AnonymousFetchUpstreamAccessToken(ctx, sd.ID, clientID)
	})
	return s.Val, s.Err
}

// UpstreamRefreshToken returns the upstream refresh token for clientID, fetching
// it from KVDB on first call and caching the result for the rest of the
// request. Concurrent calls for the same clientID share a single fetch.
func (sd *AnonymousSessionData) UpstreamRefreshToken(ctx context.Context, clientID string) (string, *errs.Error) {
	slot, _ := sd.upRTknSlots.LoadOrStore(clientID, &fwupstream.TknSlot{})
	s := slot.(*fwupstream.TknSlot)
	s.Once.Do(func() {
		s.Val, s.Err = sd.Mgr.AnonymousFetchUpstreamRefreshToken(ctx, sd.ID, clientID)
	})
	return s.Val, s.Err
}

// UpstreamRequestWithBearer is the foundation method for sending a request with
// an access token from the session. Returns the raw *http.Response (caller closes
// body) on success. res.Body is io.ReadCloser — the byte stream; consume or pass through.
func (sd *AnonymousSessionData) UpstreamRequestWithBearer(
	ctx context.Context,
	fwClient *fwupstream.Client,
	method, endpoint string,
	payload *fwupstream.RequestPayload,
) (*http.Response, int, *errs.Error) {
	accessTkn, resErr := sd.UpstreamAccessToken(ctx, fwClient.ID)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	return fwClient.RequestWithBearer(ctx, accessTkn, method, endpoint, payload)
}

// UpstreamRequestWithBearerRetriable calls UpstreamRequestWithBearer; on a
// refresh-worthy auth failure (token missing locally, or upstream rejected it),
// refreshes the token pair via fwClient.RequestRefreshAccessTknPair, updates
// the session row + cached slots, and retries once.
//
// Refresh-request body extras come from the typed refresh sideloader registered
// on the Client for this session-data type (via cookie.SetAnonymousRefreshSideloader).
// If no sideloader is registered, returns UpstreamRefreshSideloaderNotSet.
//
// res.Body is io.ReadCloser — the byte stream; consume or pass through.
func (sd *AnonymousSessionData) UpstreamRequestWithBearerRetriable(
	ctx context.Context,
	fwClient *fwupstream.Client,
	method, endpoint string,
	payload *fwupstream.RequestPayload,
) (*http.Response, int, *errs.Error) {
	res, status, resErr := sd.UpstreamRequestWithBearer(ctx, fwClient, method, endpoint, payload)
	if resErr == nil {
		return res, status, nil
	}

	shouldRefresh := resErr.IsSameCode(errs.UpstreamAccessTokenNotFound) ||
		resErr.IsSameCode(errs.AccessTokenNotFound) ||
		resErr.IsSameCode(errs.InvalidAccessToken)
	if !shouldRefresh {
		return nil, status, resErr
	}

	refreshTkn, resErr := sd.UpstreamRefreshToken(ctx, fwClient.ID)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	refreshSideloader, ok := GetAnonymousRefreshSideloader(fwClient)
	if !ok {
		return nil, http.StatusInternalServerError, errs.UpstreamRefreshSideloaderNotSet.WithDetail("Client " + fwClient.ID + " has no AnonymousRefreshSideloader")
	}
	// sd is this method's receiver — forwarded so the closure body never has to fetch session data from ctx.
	refreshExtra := refreshSideloader(ctx, sd)
	newPair, resErr := fwClient.RequestRefreshAccessTknPair(ctx, refreshTkn, refreshExtra)
	if resErr != nil {
		return nil, http.StatusUnauthorized, resErr
	}
	if resErr := sd.Mgr.AnonymousStoreUpstreamTokenPair(ctx, sd.ID, fwClient.ID, newPair.AccessToken, newPair.RefreshToken); resErr != nil {
		return nil, http.StatusInternalServerError, resErr
	}

	// Update cached lazy slots so the retry picks up the new tokens.
	sd.upATknSlots.Store(fwClient.ID, fwupstream.NewDoneTknSlot(newPair.AccessToken))
	sd.upRTknSlots.Store(fwClient.ID, fwupstream.NewDoneTknSlot(newPair.RefreshToken))

	return sd.UpstreamRequestWithBearer(ctx, fwClient, method, endpoint, payload)
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
