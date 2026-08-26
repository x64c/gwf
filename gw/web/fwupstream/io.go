package fwupstream

import (
	"context"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

// Upstream tokens are stored as hash fields on a session row at rowKey (see
// funcs.go for the field-name schema), not as independent keys. Callers must
// ensure rowKey refers to a hash type (e.g. the cookie/bearer session base row).
//
// Tokens are encrypted at rest with the Hub's TokenCipher: a KVDB dump (backup,
// managed-Redis read, separate-host compromise) yields ciphertext an attacker
// can't replay against the upstream. Each value is bound to its rowKey + field
// as cipher context, so a ciphertext copied to any other row or field no longer
// decrypts. The cipher is required — a nil Hub or nil TokenCipher means the app
// didn't configure token_cipher in .fwupstream-web.json, so these return
// UpstreamTokenCipherNotSet rather than storing plaintext.

// FetchAccessToken reads and decrypts the upstream access token for clientID
// from the session row at rowKey.
func (h *Hub) FetchAccessToken(ctx context.Context, rowKey, clientID string) (string, *errs.Error) {
	if h == nil || h.TokenCipher == nil {
		return "", errs.UpstreamTokenCipherNotSet
	}
	val, found, err := h.KVDB.HashGetField(ctx, rowKey, AccessTokenField(clientID))
	if err != nil {
		return "", errs.KVDB.WithDetail("fetching upstream access token").WithCause(err)
	}
	if !found {
		return "", errs.UpstreamAccessTokenNotFound
	}
	plaintext, err := h.TokenCipher.DecodeDecrypt(val, security.CipherContext{Location: rowKey, Field: AccessTokenField(clientID)})
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
	val, found, err := h.KVDB.HashGetField(ctx, rowKey, RefreshTokenField(clientID))
	if err != nil {
		return "", errs.KVDB.WithDetail("fetching upstream refresh token").WithCause(err)
	}
	if !found {
		return "", errs.UpstreamRefreshTokenNotFound
	}
	plaintext, err := h.TokenCipher.DecodeDecrypt(val, security.CipherContext{Location: rowKey, Field: RefreshTokenField(clientID)})
	if err != nil {
		return "", errs.InvalidUpstreamRefreshToken.WithDetail("decrypting at-rest upstream refresh token").WithCause(err)
	}
	return string(plaintext), nil
}

// StoreTokenPair encrypts and writes the access + refresh tokens for clientID
// atomically as fields on the session row at rowKey, and only if that row still
// exists. Its lifetime is left as it is — these fields inherit the row's, and
// have none of their own.
//
// The write is conditional because the row can end between the request being
// authenticated and the upstream refresh returning, which is a wide window: a
// round trip to the upstream server sits inside it. An unconditional write
// would recreate the row, and a row created this way carries NO lifetime at all
// — leaving encrypted upstream credentials with nothing to expire them and no
// session to own them.
func (h *Hub) StoreTokenPair(ctx context.Context, rowKey, clientID, accessTkn, refreshTkn string) *errs.Error {
	if h == nil || h.TokenCipher == nil {
		return errs.UpstreamTokenCipherNotSet
	}
	encAccess, err := h.TokenCipher.EncryptEncode([]byte(accessTkn), security.CipherContext{Location: rowKey, Field: AccessTokenField(clientID)})
	if err != nil {
		return errs.Upstream.WithDetail("encrypting upstream access token").WithCause(err)
	}
	encRefresh, err := h.TokenCipher.EncryptEncode([]byte(refreshTkn), security.CipherContext{Location: rowKey, Field: RefreshTokenField(clientID)})
	if err != nil {
		return errs.Upstream.WithDetail("encrypting upstream refresh token").WithCause(err)
	}
	fields := map[string]any{
		AccessTokenField(clientID):  encAccess,
		RefreshTokenField(clientID): encRefresh,
	}
	existed, err := h.KVDB.HashSetFieldsIfExists(ctx, rowKey, fields)
	if err != nil {
		return errs.KVDB.WithDetail("storing upstream token pair").WithCause(err)
	}
	if !existed {
		return errs.Upstream.WithDetail("session row ended before its upstream tokens could be stored")
	}
	return nil
}

// StoreAccessToken writes the encrypted access token for clientID on the
// row and removes the row's refresh-token field for clientID: the shape of
// a token that has no refresh token (a user token by machine assertion).
// Same conditional write as StoreTokenPair — nothing is written to a row
// that has ended.
func (h *Hub) StoreAccessToken(ctx context.Context, rowKey, clientID, accessTkn string) *errs.Error {
	if h == nil || h.TokenCipher == nil {
		return errs.UpstreamTokenCipherNotSet
	}
	encAccess, err := h.TokenCipher.EncryptEncode([]byte(accessTkn), security.CipherContext{Location: rowKey, Field: AccessTokenField(clientID)})
	if err != nil {
		return errs.Upstream.WithDetail("encrypting upstream access token").WithCause(err)
	}
	existed, err := h.KVDB.HashSetFieldIfExists(ctx, rowKey, AccessTokenField(clientID), encAccess)
	if err != nil {
		return errs.KVDB.WithDetail("storing upstream access token").WithCause(err)
	}
	if !existed {
		return errs.Upstream.WithDetail("session row ended before its upstream token could be stored")
	}
	if _, err = h.KVDB.HashRemoveFields(ctx, rowKey, RefreshTokenField(clientID)); err != nil {
		return errs.KVDB.WithDetail("removing upstream refresh token field").WithCause(err)
	}
	return nil
}

// RemoveTokens removes the row's access- and refresh-token fields for
// clientID. The row itself is untouched.
func (h *Hub) RemoveTokens(ctx context.Context, rowKey, clientID string) *errs.Error {
	if h == nil {
		return errs.UpstreamTokenCipherNotSet
	}
	if _, err := h.KVDB.HashRemoveFields(ctx, rowKey, AccessTokenField(clientID), RefreshTokenField(clientID)); err != nil {
		return errs.KVDB.WithDetail("removing upstream token fields").WithCause(err)
	}
	return nil
}
