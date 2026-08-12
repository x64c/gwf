package fwupstream

import (
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
)

// Hub is the app's whole upstream subsystem: the configured clients and the
// at-rest upstream-token store (KVDB + cipher). It is held once on the framework
// Core (c.FWUpstream) and referenced by the session managers, which delegate
// upstream-token I/O to it.
//
// One nested object, rather than these fields spread onto each session manager,
// because one conf describes the whole subsystem and every session protocol
// draws on the same set of upstreams: .fwupstream-web.json holds the cipher and
// the per-Client configs together, PrepareFWUpstream builds them once, and each
// session manager takes a reference. The subsystem is also shared and stateful,
// which is what makes a reference the only workable shape:
//
//   - Both managers point at the SAME Hub — PrepareBearerSessions and
//     PrepareCookieSessions each assign c.FWUpstream. Flattened, each would hold
//     its own copy of a registry that has to stay identical, with nothing to
//     keep it so.
//   - A Client is shared state, not config. It embeds *http.Client with its
//     connection pool, and a registry of refresh sideloaders keyed BY SESSION
//     DATA TYPE — bearer and cookie register their own closures on the same
//     Client under different keys. That only works with one instance of it.
//   - Absence is one fact. An app with no upstream has c.FWUpstream == nil, and
//     the session Prepares fail loud on it; flattened, absence would be three
//     fields that must be empty together and nothing to check at once.
//
// Delegation, not trespass: a session manager owns its rows and hands the Hub
// the key of the one it wants written. The Hub takes an opaque rowKey and has no
// view of session lifecycle — which is why its writes are conditional (see
// StoreTokenPair), so delegating can never create a row the session layer
// considers gone.
type Hub struct {
	Clients     map[string]*Client     // configured upstream clients, by id
	KVDB        kvdbs.DB               // MainKVDB — holds the session rows the tokens live on
	TokenCipher security.EncodedCipher // at-rest cipher for upstream tokens; nil if the app stores no upstream tokens
}
