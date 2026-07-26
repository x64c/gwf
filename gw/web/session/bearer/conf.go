package bearer

// SessionGroupConf is one bearer session-management group — one top-level entry
// in .bearer-session.json. All clients within a group share the group's session
// policy (binds, cap, TTLs).
//
// Identity dimensions:
//   - Binds: identity binding labels for sessions in this group, e.g.
//     ["client", "user"]. Must include "client" since every group has a
//     non-empty Clients map.
//
// A group may contain a "" client entry representing the clientless case
// (Client-Id header absent). Across the entire config, at most one client name
// may be "", and it counts as a normal SessionClientConf — same group policy
// applies to it.
type SessionGroupConf struct {
	Name            string                        `json:"-"`                 // populated from JSON top-level key at boot; for diagnostics
	Binds           []string                      `json:"binds"`             // identity binding labels, e.g. ["client", "user"]
	Cap             *SessionCapConf               `json:"cap,omitempty"`     // nil = no cap
	AccessTTL       int                           `json:"access_ttl"`        // seconds
	RefreshTTL      int                           `json:"refresh_ttl"`       // seconds (sliding)
	RefreshChainTTL int                           `json:"refresh_chain_ttl"` // seconds (absolute lifetime of the refresh chain)
	Clients         map[string]*SessionClientConf `json:"clients"`           // keyed by client name (from JSON sub-key); non-empty
}

// SessionCapConf is the cap rule for a session-management group. The bucket
// key is the tuple of values for the axes listed in By. Max is the max number
// of concurrent sessions allowed per bucket.
//
// By must be a non-empty subset of the parent group's Binds.
type SessionCapConf struct {
	Max int      `json:"max"`
	By  []string `json:"by"`
}

// SessionClientConf is one registered downstream client within a session group.
//
// ID is the value sent in the Client-Id header; "" denotes the clientless
// member of the group (recognized when no Client-Id header is present). Across
// the entire config, all ID values are globally unique — duplicate IDs are a
// fatal boot error.
type SessionClientConf struct {
	Name          string            `json:"-"`               // populated from JSON clients-map key at boot; for diagnostics
	ID            string            `json:"id"`              // Client-Id header value; "" = clientless
	ExtAuthSecret string            `json:"ext_auth_secret"` // external auth provider secret (OAuth client secret, etc.)
	Group         *SessionGroupConf `json:"-"`               // back-ref to parent group, set at boot
}
