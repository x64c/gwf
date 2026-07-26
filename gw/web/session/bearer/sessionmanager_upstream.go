package bearer

import (
	"context"

	"github.com/x64c/gwf/gw/errs"
)

// FetchUpstreamAccessToken reads the upstream access token for the given
// clientID from the bearer session umbrella row.
func (m *SessionManager) FetchUpstreamAccessToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchAccessToken(ctx, m.SessionRowKey(sid), clientID)
}

// FetchUpstreamRefreshToken reads the upstream refresh token for the given
// clientID from the bearer session umbrella row.
func (m *SessionManager) FetchUpstreamRefreshToken(ctx context.Context, sid, clientID string) (string, *errs.Error) {
	return m.FWUpstream.FetchRefreshToken(ctx, m.SessionRowKey(sid), clientID)
}

// StoreUpstreamTokenPair writes the access + refresh tokens for the given
// clientID atomically as fields on the bearer session umbrella row. The row's
// existing TTL is preserved (children inherit lifetime from the parent).
func (m *SessionManager) StoreUpstreamTokenPair(ctx context.Context, sid, clientID, accessTkn, refreshTkn string) *errs.Error {
	return m.FWUpstream.StoreTokenPair(ctx, m.SessionRowKey(sid), clientID, accessTkn, refreshTkn)
}
