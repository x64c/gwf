package cookie

import (
	"context"

	"github.com/x64c/gwf/gw/errs"
)

// UserFetchUpstreamAccessToken reads the upstream access token for the given
// clientID from the user cookie session row.
func (m *SessionManager) UserFetchUpstreamAccessToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchAccessToken(ctx, m.UserSessionRowKey(sid), clientID)
}

// AnonymousFetchUpstreamAccessToken reads the upstream access token for the given
// clientID from the anonymous cookie session row.
func (m *SessionManager) AnonymousFetchUpstreamAccessToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchAccessToken(ctx, m.AnonymousSessionRowKey(sid), clientID)
}

// UserFetchUpstreamRefreshToken reads the upstream refresh token for the given
// clientID from the user cookie session row.
func (m *SessionManager) UserFetchUpstreamRefreshToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchRefreshToken(ctx, m.UserSessionRowKey(sid), clientID)
}

// AnonymousFetchUpstreamRefreshToken reads the upstream refresh token for the given
// clientID from the anonymous cookie session row.
func (m *SessionManager) AnonymousFetchUpstreamRefreshToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchRefreshToken(ctx, m.AnonymousSessionRowKey(sid), clientID)
}

// UserStoreUpstreamTokenPair writes the access + refresh tokens for the given
// clientID atomically as fields on the user cookie session row. The session row's
// existing TTL is preserved (children inherit lifetime from the parent).
func (m *SessionManager) UserStoreUpstreamTokenPair(ctx context.Context, sid, clientID, accessTkn, refreshTkn string) *errs.Error {
	return m.FWUpstream.StoreTokenPair(ctx, m.UserSessionRowKey(sid), clientID, accessTkn, refreshTkn)
}

// AnonymousStoreUpstreamTokenPair writes the access + refresh tokens for the given
// clientID atomically as fields on the anonymous cookie session row. The session row's
// existing TTL is preserved (children inherit lifetime from the parent).
func (m *SessionManager) AnonymousStoreUpstreamTokenPair(ctx context.Context, sid, clientID, accessTkn, refreshTkn string) *errs.Error {
	return m.FWUpstream.StoreTokenPair(ctx, m.AnonymousSessionRowKey(sid), clientID, accessTkn, refreshTkn)
}
