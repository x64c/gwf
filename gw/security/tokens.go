package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type AccessTokenPair struct {
	AccessToken  string
	RefreshToken string
}

// GenerateOpaqueAccessRefreshTokenPair returns two independently-random,
// 256-bit, URL-safe base64-encoded opaque tokens.
func GenerateOpaqueAccessRefreshTokenPair() (string, string) {
	return GenerateBase64RawURL(32), GenerateBase64RawURL(32)
}

// GenerateRSASignedJWTIDToken generates a jwt id_token signed by RS256
// sub: User ID
// email: Email Used for Authentication
func GenerateRSASignedJWTIDToken(iss string, sub string, email string, clientID string, privateKey *rsa.PrivateKey, kid string, expDuration time.Duration) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":   sub,
		"email": email,
		"iat":   now.Unix(),
		"exp":   now.Add(expDuration).Unix(),
		"iss":   iss,
		"aud":   clientID,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid
	return token.SignedString(privateKey)
}

// VerifyRSASignedIDToken verifies an RS256-signed JWT ID token end-to-end:
//   - signature against pubKey (PSS-style RS256)
//   - signing method is RSA (rejects "none" / HMAC tokens)
//   - exp claim is present and in the future
//   - aud claim equals expectedAud (the verifying client's id)
//   - iss claim equals expectedIss (the expected issuer host)
//
// Returns the validated claim map on success. Use this for every ID-token
// verification; do not roll your own signature-only check.
func VerifyRSASignedIDToken(signedToken string, pubKey *rsa.PublicKey, expectedAud string, expectedIss string) (jwt.MapClaims, error) {
	parsedToken, err := jwt.Parse(
		signedToken,
		func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			return pubKey, nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithAudience(expectedAud),
		jwt.WithIssuer(expectedIss),
	)
	if err != nil {
		return nil, err
	}
	if !parsedToken.Valid {
		return nil, errors.New("invalid token")
	}
	claims, ok := parsedToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("failed to convert token claims to a map")
	}
	return claims, nil
}

// GenerateBase64RawURL returns a URL-safe base64-encoded crypto-random string (no padding).
// byteSize is the source-byte count (= bits of entropy / 8).
// Output length = ceil(byteSize * 4 / 3) chars. Examples: byteSize=16 → 22 chars; byteSize=32 → 43 chars.
func GenerateBase64RawURL(byteSize int) string {
	bytes := make([]byte, byteSize)
	_, _ = rand.Read(bytes) // crypto/rand.Read never errors (Go 1.24+)
	return base64.RawURLEncoding.EncodeToString(bytes)
}

// GenerateHex returns a hex-encoded crypto-random string.
// byteSize is the source-byte count (= bits of entropy / 8).
// Output length = byteSize * 2 chars. Examples: byteSize=16 → 32 chars; byteSize=32 → 64 chars.
func GenerateHex(byteSize int) string {
	bytes := make([]byte, byteSize)
	_, _ = rand.Read(bytes) // crypto/rand.Read never errors (Go 1.24+)
	return hex.EncodeToString(bytes)
}

func HashHexSHA256(data string) string {
	// SHA256 checksum (digest) of the data
	checksum := sha256.Sum256([]byte(data))
	// hexadecimal encoding
	return hex.EncodeToString(checksum[:])
}
