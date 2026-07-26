package fwupstream

import (
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
)

// Hub is the app's whole upstream subsystem: the configured clients and the
// at-rest upstream-token store (KVDB + cipher). It is held once on the framework
// Core (c.FWUpstream) and referenced by the session managers, which delegate
// upstream-token I/O to it.
type Hub struct {
	Clients     map[string]*Client     // configured upstream clients, by id
	KVDB        kvdbs.DB               // MainKVDB — holds the session rows the tokens live on
	TokenCipher security.EncodedCipher // at-rest cipher for upstream tokens; nil if the app stores no upstream tokens
}
