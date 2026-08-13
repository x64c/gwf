package cookie

import (
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/security"
)

type ExpireMode string

// The two expiry policies are alternatives, chosen per shape: ExpireAbsolute
// sets the session's TTL once at creation — it ends ExpireIn seconds later
// regardless of activity — while ExpireSliding extends the TTL on activity
// (see ExtendThreshold), so a session in regular use stays alive and only an
// idle gap of ExpireIn seconds ends it.
//
// There is deliberately no third policy stacking an absolute TTL on top of
// sliding. Sliding's contract is "activity buys a full ExpireIn from now";
// under an absolute TTL, the last extension can only deliver whatever remains
// of it — an arbitrarily short stub, handed out silently. Cookie
// sessions carry a human at a browser, and that hybrid fires on exactly the
// most regular users: today's visit looks like every other visit, the
// extension appears to fire, and the session ends minutes in with no visible
// reason. A deployment that wants hard-scheduled logout chooses
// absolute; one that wants activity-based lifetime chooses sliding and bounds
// it with the idle timeout. (Bearer's refresh_chain_ttl caps a machine
// credential — the API client re-authenticates programmatically and no human
// sees it. The asymmetry is per principal type, and intentional.)
//
//	simple absolute — ok:
//	|start-------------------absttl------------------ends|
//
//	sliding with absttl — the rejected hybrid:
//	|start---------|.............absttl..............|
//	          |ext---------|
//	                     |ext----------|
//	                                |ext-----------|
//	                                              |--|  ← today's "extension"
//	                                                      is a stub: kicked out
//	                                                      after a very short
//	                                                      period of normal
//	                                                      daily use
const (
	ExpireAbsolute ExpireMode = "absolute"
	ExpireSliding  ExpireMode = "sliding"
)

// SessionConf is the cookie session configuration with optional per-shape sub-confs.
// Each shape's sub-conf is nil if the app doesn't use that shape. One keyring
// serves both shapes: each shape's cipher is HKDF-derived from it under its
// own purpose label, so the two are cryptographically distinct.
type SessionConf struct {
	Keyring          *security.KeyringConf
	UserSession      *UserSessionConf
	AnonymousSession *AnonymousSessionConf
}

// HKDF purpose labels the cookie ciphers are derived under — distinct from
// every other purpose even when keyring master keys are shared or reused
// across config sections.
const (
	UserCookieCipherPurpose      = "user-cookie"
	AnonymousCookieCipherPurpose = "anonymous-cookie"
)

// UserSessionConf is the cookie session config for the user-bound shape (logged-in users).
type UserSessionConf struct {
	ExpireIn           int        `json:"expire_in"` // seconds
	ExpireMode         ExpireMode `json:"expire_mode"`
	ExtendThreshold    int        `json:"extend_threshold"` // seconds; for sliding
	LoginPath          string     `json:"login_path"`
	MaxSessionsPerUser int64      `json:"max_sessions_per_user"`
}

// AnonymousSessionConf is the cookie session config for the anonymous shape —
// sessions for visitors not logged in (cart, A/B bucket, view tracker, etc.).
type AnonymousSessionConf struct {
	ExpireIn        int        `json:"expire_in"` // seconds
	ExpireMode      ExpireMode `json:"expire_mode"`
	ExtendThreshold int        `json:"extend_threshold"` // seconds; for sliding
}

// validateExpiry checks the expiry fields shared by both shapes. label prefixes
// every error message (e.g. "user cookie session"). ExtendThreshold is only
// meaningful — and only checked — in sliding mode; it is ignored in absolute mode.
func validateExpiry(label string, expireIn int, mode ExpireMode, extendThreshold int) error {
	if expireIn <= 0 {
		return fmt.Errorf("%s: expire_in must be > 0 (got %d)", label, expireIn)
	}
	switch mode {
	case ExpireAbsolute, ExpireSliding:
		// ok
	default:
		return fmt.Errorf("%s: expire_mode must be %q or %q (got %q)", label, ExpireAbsolute, ExpireSliding, mode)
	}
	if mode == ExpireSliding && (extendThreshold <= 0 || extendThreshold >= expireIn) {
		return fmt.Errorf("%s: extend_threshold must be in (0, %d) for sliding mode (got %d)", label, expireIn, extendThreshold)
	}
	return nil
}

// Validate reports the first invalid field in the user-shape config, so a
// misconfigured .web-cookie-session.json fails loudly at startup rather than
// surfacing as obscure runtime behavior. The keyring is validated separately
// during cipher construction.
func (c *UserSessionConf) Validate() error {
	if err := validateExpiry("user cookie session", c.ExpireIn, c.ExpireMode, c.ExtendThreshold); err != nil {
		return err
	}
	if c.LoginPath == "" {
		return fmt.Errorf("user cookie session: login_path must not be empty")
	}
	if !strings.HasPrefix(c.LoginPath, "/") {
		return fmt.Errorf("user cookie session: login_path must begin with '/' (got %q)", c.LoginPath)
	}
	if c.MaxSessionsPerUser < 0 {
		return fmt.Errorf("user cookie session: max_sessions_per_user must be >= 0 (got %d)", c.MaxSessionsPerUser)
	}
	return nil
}

// Validate reports the first invalid field in the anonymous-shape config. The
// anonymous shape has no LoginPath or MaxSessionsPerUser, so only the shared
// expiry fields are checked.
func (c *AnonymousSessionConf) Validate() error {
	return validateExpiry("anonymous cookie session", c.ExpireIn, c.ExpireMode, c.ExtendThreshold)
}
