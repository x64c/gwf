package jwtassert

import (
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/x64c/gwf/gw/security"
)

// Signer is the client half: it mints one assertion per request with the
// client's private key.
type Signer struct {
	ID         string          // this client's id: `iss` and `sub`
	Audience   string          // the verifying side's identity: `aud`
	Kid        string          // key id of PrivateKey, as the verifier knows it
	PrivateKey *rsa.PrivateKey // never leaves this host
	MaxAge     time.Duration   // `exp` − `iat`; must not exceed the verifier's MaxAge
}

// Sign returns the compact JWS for one request. requestTarget is the path
// plus raw query exactly as the request will carry it; body is the request
// body (nil when none). extra claims are added as given; claims this package
// defines win on collision.
func (s *Signer) Sign(method, requestTarget string, body []byte, extra map[string]any) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{}
	for k, v := range extra {
		claims[k] = v
	}
	claims["iss"] = s.ID
	claims["sub"] = s.ID
	claims["aud"] = s.Audience
	claims["iat"] = now.Unix()
	claims["exp"] = now.Add(s.MaxAge).Unix()
	claims["jti"] = security.GenerateBase64RawURL(16)
	claims[ClaimHTTPMethod] = method
	claims[ClaimHTTPTarget] = requestTarget
	if body != nil {
		claims[ClaimBodyHash] = BodyHash(body)
	} else {
		delete(claims, ClaimBodyHash)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.Kid
	return token.SignedString(s.PrivateKey)
}

// AuthorizationValue returns the Authorization header value for a signed
// assertion.
func AuthorizationValue(signed string) string {
	return AuthScheme + " " + signed
}

// BodyHash is the `body_hash` claim value for a request body: base64url
// (no padding) of its SHA-256.
func BodyHash(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
