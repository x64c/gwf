package bearer

import (
	"context"

	"github.com/x64c/gwf/gw/web/fwupstream"
)

// UserRefreshSideloader returns extra body fields to merge into the
// token-refresh request for an upstream Client, given the bearer
// user-session data and the request context.
//
// The closure is invoked by UserSessionData[UID].UpstreamRequestWithBearerRetriable
// during the refresh path. The Retriable method passes its own receiver as sd,
// so the closure body never has to fetch session data from ctx.
type UserRefreshSideloader[UID comparable] = func(ctx context.Context, sd *UserSessionData[UID]) map[string]any

type userRefreshSideloaderKey struct{}

// SetUserRefreshSideloader registers a refresh sideloader on c for the bearer
// UserSessionData type. The UID type parameter must match the app's chosen
// UID type.
func SetUserRefreshSideloader[UID comparable](c *fwupstream.Client, fn UserRefreshSideloader[UID]) {
	fwupstream.SetRefreshSideloader(c, userRefreshSideloaderKey{}, fn)
}

// GetUserRefreshSideloader retrieves the refresh sideloader registered on c
// for the bearer UserSessionData type.
func GetUserRefreshSideloader[UID comparable](c *fwupstream.Client) (UserRefreshSideloader[UID], bool) {
	return fwupstream.GetRefreshSideloader[UserRefreshSideloader[UID]](c, userRefreshSideloaderKey{})
}

// UserlessRefreshSideloader returns extra body fields to merge into the
// token-refresh request for an upstream Client, given the bearer
// userless-session data and the request context.
//
// The closure is invoked by UserlessSessionData.UpstreamRequestWithBearerRetriable
// during the refresh path. The Retriable method passes its own receiver as sd,
// so the closure body never has to fetch session data from ctx.
type UserlessRefreshSideloader = func(ctx context.Context, sd *UserlessSessionData) map[string]any

type userlessRefreshSideloaderKey struct{}

// SetUserlessRefreshSideloader registers a refresh sideloader on c for the
// bearer UserlessSessionData type.
func SetUserlessRefreshSideloader(c *fwupstream.Client, fn UserlessRefreshSideloader) {
	fwupstream.SetRefreshSideloader(c, userlessRefreshSideloaderKey{}, fn)
}

// GetUserlessRefreshSideloader retrieves the refresh sideloader registered on
// c for the bearer UserlessSessionData type.
func GetUserlessRefreshSideloader(c *fwupstream.Client) (UserlessRefreshSideloader, bool) {
	return fwupstream.GetRefreshSideloader[UserlessRefreshSideloader](c, userlessRefreshSideloaderKey{})
}
