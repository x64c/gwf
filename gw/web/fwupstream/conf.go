package fwupstream

import "github.com/x64c/gwf/gw/security"

// Conf is the parsed `.fwupstream-web.json` — the app's whole upstream subsystem
// config: the optional at-rest token keyring plus the per-Client configs.
type Conf struct {
	TokenCipher *security.KeyringConf  `json:"token_cipher"` // optional; present iff the app stores upstream tokens (absent for JWKS-only upstreams)
	Clients     map[string]*ClientConf `json:"clients"`      // per-Client config, keyed by Client id
}

// TokenCipherPurpose is the HKDF purpose label the upstream token cipher is
// derived under — distinct from every other purpose even when keyring master
// keys are shared or reused across config sections.
const TokenCipherPurpose = "upstream-token"
