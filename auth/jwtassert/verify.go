package jwtassert

import (
	"bytes"
	"crypto/subtle"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/x64c/gwf/gw/authn"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/security"
)

// Verifier is the receiving half: it authenticates requests carrying
// assertions from configured clients. Build it with NewVerifier; use one
// long-lived Verifier — its ReplayStore is what makes an assertion single-use,
// and a Verifier built per request would remember nothing.
type Verifier struct {
	byID   map[string]*Client // by Client.ID — the assertion's `iss`
	replay ReplayStore
}

// NewVerifier takes the clients as configured — keyed by human name — stamps
// each Client.Name from its key, validates it, loads its keys, and indexes the
// clients by ID. Duplicate or empty ids fail construction: the id is what an
// assertion names, so it must be unique.
//
// replay is required and is named by the caller: how far a replay window
// reaches is a deployment's answer, not this package's, so there is nothing
// sensible to pick on the caller's behalf. NewReplayWindow covers one process.
func NewVerifier(clients map[string]*Client, replay ReplayStore) (*Verifier, error) {
	if replay == nil {
		return nil, errors.New("jwtassert.NewVerifier: replay store required")
	}
	v := &Verifier{byID: make(map[string]*Client, len(clients)), replay: replay}
	for name, p := range clients {
		p.Name = name
		if err := p.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if other, dup := v.byID[p.ID]; dup {
			return nil, fmt.Errorf("jwtassert clients %s and %s share id %q", other.Name, name, p.ID)
		}
		if err := p.LoadKeys(); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		v.byID[p.ID] = p
	}
	return v, nil
}

// VerifyRequest authenticates r by the assertion in its Authorization
// header. On success the identity's Subject is the client's ID (the value the
// assertion carried and proved), Claims are the assertion's claims, and the
// matched Client is returned for its Name. The request body, when present, is
// consumed for hashing and replaced so handlers read it unchanged.
//
// Every failure is one of errs.AssertionNotFound, errs.AssertionClientUnknown,
// errs.InvalidAssertion (detail says which check), errs.AssertionReplayed,
// errs.AssertionReplayUnknown, or errs.RequestBodyTooLarge.
func (v *Verifier) VerifyRequest(r *http.Request) (authn.VerifiedIdentity, *Client, *errs.Error) {
	signed, e := assertionFromHeader(r)
	if e != nil {
		return authn.VerifiedIdentity{}, nil, e
	}

	kid, issuer, e := peekHeaderAndIssuer(signed)
	if e != nil {
		return authn.VerifiedIdentity{}, nil, e
	}
	client, ok := v.byID[issuer]
	if !ok {
		return authn.VerifiedIdentity{}, nil, errs.AssertionClientUnknown.WithDetail(issuer)
	}
	jwk, err := client.jwks.GetJWKByKID(kid)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("unknown kid " + kid)
	}
	pubKey, err := jwk.ToPublicKey()
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.Wrap(err)
	}

	skew := time.Duration(client.ClockSkew) * time.Second
	parsed, err := jwt.Parse(
		signed,
		func(*jwt.Token) (any, error) { return pubKey, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithIssuer(issuer),
		jwt.WithSubject(issuer),
		jwt.WithAudience(client.Audience),
		jwt.WithLeeway(skew),
	)
	if err != nil {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.Wrap(err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("claims")
	}

	iat, err := claims.GetIssuedAt()
	if err != nil || iat == nil {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("iat required")
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("exp required")
	}
	if exp.Sub(iat.Time) > time.Duration(client.MaxAge)*time.Second {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("lifetime exceeds max_age")
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("jti required")
	}

	if got, _ := claims[ClaimHTTPMethod].(string); got != r.Method {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("htm mismatch")
	}
	if got, _ := claims[ClaimHTTPTarget].(string); got != r.URL.RequestURI() {
		return authn.VerifiedIdentity{}, nil, errs.InvalidAssertion.WithDetail("htu mismatch")
	}

	if e = verifyBody(r, client.MaxBodyBytes, claims); e != nil {
		return authn.VerifiedIdentity{}, nil, e
	}

	admitted, err := v.replay.Admit(r.Context(), issuer+"\x00"+jti, exp.Add(skew))
	if err != nil {
		// Nothing was decided about this assertion, so it is not cleared.
		return authn.VerifiedIdentity{}, nil, errs.AssertionReplayUnknown.Wrap(err)
	}
	if !admitted {
		return authn.VerifiedIdentity{}, nil, errs.AssertionReplayed
	}

	return authn.VerifiedIdentity{
		Method:  Method,
		Subject: client.ID,
		Claims:  map[string]any(claims),
	}, client, nil
}

func assertionFromHeader(r *http.Request) (string, *errs.Error) {
	header := r.Header.Get("Authorization")
	if header == "" {
		return "", errs.AssertionNotFound
	}
	scheme, signed, found := strings.Cut(header, " ")
	if !found || !strings.EqualFold(scheme, AuthScheme) {
		return "", errs.AssertionNotFound
	}
	signed = strings.TrimSpace(signed)
	if signed == "" {
		return "", errs.AssertionNotFound
	}
	return signed, nil
}

// peekHeaderAndIssuer reads kid and iss BEFORE verification — only to pick
// the client and key. Both are re-established by the verified parse.
func peekHeaderAndIssuer(signed string) (kid, issuer string, e *errs.Error) {
	encodedHeader, encodedPayload, _, err := security.SplitSignedJwtTokenRawParts(signed)
	if err != nil {
		return "", "", errs.InvalidAssertion.WithDetail("malformed")
	}
	headerBytes, err := security.DecodeJwtHeader(encodedHeader)
	if err != nil {
		return "", "", errs.InvalidAssertion.WithDetail("malformed header")
	}
	var header struct {
		Kid string `json:"kid"`
	}
	if err = json.Unmarshal(headerBytes, &header); err != nil || header.Kid == "" {
		return "", "", errs.InvalidAssertion.WithDetail("kid required")
	}
	payloadBytes, err := security.DecodeJwtHeader(encodedPayload) // same base64url raw decoding
	if err != nil {
		return "", "", errs.InvalidAssertion.WithDetail("malformed payload")
	}
	var payload struct {
		Iss string `json:"iss"`
	}
	if err = json.Unmarshal(payloadBytes, &payload); err != nil || payload.Iss == "" {
		return "", "", errs.InvalidAssertion.WithDetail("iss required")
	}
	return header.Kid, payload.Iss, nil
}

// verifyBody hashes the request body (if any) against the body_hash claim
// and puts the bytes back on r.Body.
func verifyBody(r *http.Request, maxBytes int64, claims jwt.MapClaims) *errs.Error {
	var body []byte
	if r.Body != nil && r.Body != http.NoBody {
		b, err := io.ReadAll(io.LimitReader(r.Body, maxBytes+1))
		_ = r.Body.Close()
		if err != nil {
			return errs.InvalidAssertion.Wrap(fmt.Errorf("reading body: %w", err))
		}
		if int64(len(b)) > maxBytes {
			return errs.RequestBodyTooLarge
		}
		body = b
		r.Body = io.NopCloser(bytes.NewReader(body))
	}

	claimed, hasClaim := claims[ClaimBodyHash].(string)
	switch {
	case len(body) == 0 && !hasClaim:
		return nil
	case len(body) == 0 && hasClaim:
		return errs.InvalidAssertion.WithDetail("body_hash without body")
	case !hasClaim:
		return errs.InvalidAssertion.WithDetail("body_hash required")
	}
	if subtle.ConstantTimeCompare([]byte(claimed), []byte(BodyHash(body))) != 1 {
		return errs.InvalidAssertion.WithDetail("body_hash mismatch")
	}
	return nil
}
