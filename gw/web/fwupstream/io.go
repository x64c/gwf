package fwupstream

import (
	"context"

	"github.com/x64c/gwf/gw/errs"
)

// Upstream tokens are stored as hash fields on a session row at rowKey (see
// funcs.go for the field-name schema), not as independent keys. Callers must
// ensure rowKey refers to a hash type (e.g. the cookie/bearer session base row).
//
// Tokens are encrypted at rest with the Hub's TokenCipher: a KVDB dump (backup,
// managed-Redis read, separate-host compromise) yields ciphertext an attacker
// can't replay against the upstream. The cipher is required — a nil Hub or nil
// TokenCipher means the app didn't configure token_cipher in .fwupstream-web.json, so
// these return UpstreamTokenCipherNotSet rather than storing plaintext.

// FetchAccessToken reads and decrypts the upstream access token for clientID
// from the session row at rowKey.
func (h *Hub) FetchAccessToken(ctx context.Context, rowKey, clientID string) (string, *errs.Error) {
	if h == nil || h.TokenCipher == nil {
		return "", errs.UpstreamTokenCipherNotSet
	}
	val, found, err := h.KVDB.GetField(ctx, rowKey, AccessTokenField(clientID))
	if err != nil {
		return "", errs.KVDB.WithDetail("fetching upstream access token").WithCause(err)
	}
	if !found {
		return "", errs.UpstreamAccessTokenNotFound
	}
	plaintext, err := h.TokenCipher.DecodeDecrypt(val)
	if err != nil {
		return "", errs.InvalidUpstreamAccessToken.WithDetail("decrypting at-rest upstream access token").WithCause(err)
	}
	return string(plaintext), nil
}

// FetchRefreshToken reads and decrypts the upstream refresh token for clientID
// from the session row at rowKey.
func (h *Hub) FetchRefreshToken(ctx context.Context, rowKey, clientID string) (string, *errs.Error) {
	if h == nil || h.TokenCipher == nil {
		return "", errs.UpstreamTokenCipherNotSet
	}
	val, found, err := h.KVDB.GetField(ctx, rowKey, RefreshTokenField(clientID))
	if err != nil {
		return "", errs.KVDB.WithDetail("fetching upstream refresh token").WithCause(err)
	}
	if !found {
		return "", errs.UpstreamRefreshTokenNotFound
	}
	plaintext, err := h.TokenCipher.DecodeDecrypt(val)
	if err != nil {
		return "", errs.InvalidUpstreamRefreshToken.WithDetail("decrypting at-rest upstream refresh token").WithCause(err)
	}
	return string(plaintext), nil
}

// StoreTokenPair encrypts and writes the access + refresh tokens for clientID
// atomically as fields on the session row at rowKey. The row's existing TTL is
// preserved (children inherit lifetime from the parent).
func (h *Hub) StoreTokenPair(ctx context.Context, rowKey, clientID, accessTkn, refreshTkn string) *errs.Error {
	if h == nil || h.TokenCipher == nil {
		return errs.UpstreamTokenCipherNotSet
	}
	encAccess, err := h.TokenCipher.EncryptEncode([]byte(accessTkn))
	if err != nil {
		return errs.Upstream.WithDetail("encrypting upstream access token").WithCause(err)
	}
	encRefresh, err := h.TokenCipher.EncryptEncode([]byte(refreshTkn))
	if err != nil {
		return errs.Upstream.WithDetail("encrypting upstream refresh token").WithCause(err)
	}
	fields := map[string]any{
		AccessTokenField(clientID):  encAccess,
		RefreshTokenField(clientID): encRefresh,
	}
	if err := h.KVDB.SetFields(ctx, rowKey, fields); err != nil {
		return errs.KVDB.WithDetail("storing upstream token pair").WithCause(err)
	}
	return nil
}
