package cookie

import (
	"context"

	"github.com/x64c/gwf/gw/web/fwupstream"
)

// UserRefreshSideloader returns extra body fields to merge into the
// token-refresh request for an upstream Client, given the cookie
// user-session data and the request context.
//
// The closure is invoked by UserSessionData[UID].UpstreamRequestWithBearerRetriable
// during the refresh path. The Retriable method passes its own receiver as sd,
// so the closure body never has to fetch session data from ctx.
type UserRefreshSideloader[UID comparable] = func(ctx context.Context, sd *UserSessionData[UID]) map[string]any

type userRefreshSideloaderKey struct{}

// SetUserRefreshSideloader registers a refresh sideloader on c for the cookie
// UserSessionData type. The UID type parameter must match the app's chosen
// UID type.
func SetUserRefreshSideloader[UID comparable](c *fwupstream.Client, fn UserRefreshSideloader[UID]) {
	c.SetRefreshSideloader(userRefreshSideloaderKey{}, fn)
}

// GetUserRefreshSideloader retrieves the refresh sideloader registered on c
// for the cookie UserSessionData type.
func GetUserRefreshSideloader[UID comparable](c *fwupstream.Client) (UserRefreshSideloader[UID], bool) {
	return c.GetRefreshSideloader[UserRefreshSideloader[UID]](userRefreshSideloaderKey{})
}

// AnonymousRefreshSideloader returns extra body fields to merge into the
// token-refresh request for an upstream Client, given the cookie
// anonymous-session data and the request context.
//
// The closure is invoked by AnonymousSessionData.UpstreamRequestWithBearerRetriable
// during the refresh path. The Retriable method passes its own receiver as sd,
// so the closure body never has to fetch session data from ctx.
type AnonymousRefreshSideloader = func(ctx context.Context, sd *AnonymousSessionData) map[string]any

type anonymousRefreshSideloaderKey struct{}

// SetAnonymousRefreshSideloader registers a refresh sideloader on c for the
// cookie AnonymousSessionData type.
func SetAnonymousRefreshSideloader(c *fwupstream.Client, fn AnonymousRefreshSideloader) {
	c.SetRefreshSideloader(anonymousRefreshSideloaderKey{}, fn)
}

// GetAnonymousRefreshSideloader retrieves the refresh sideloader registered
// on c for the cookie AnonymousSessionData type.
func GetAnonymousRefreshSideloader(c *fwupstream.Client) (AnonymousRefreshSideloader, bool) {
	return c.GetRefreshSideloader[AnonymousRefreshSideloader](anonymousRefreshSideloaderKey{})
}
