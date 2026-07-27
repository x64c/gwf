package fwupstream

// Conf is the parsed `.fwupstream-web.json` — the app's whole upstream subsystem
// config: the optional at-rest token cipher plus the per-Client configs.
type Conf struct {
	TokenCipher *TokenCipherConf         `json:"token_cipher"` // optional; present iff the app stores upstream tokens (absent for JWKS-only upstreams)
	Clients     map[string]*ClientConf `json:"clients"`      // per-Client config, keyed by Client id
}

// TokenCipherConf configures the at-rest cipher for upstream OAuth tokens.
type TokenCipherConf struct {
	EncKey string `json:"enckey"` // base64 (std, padded) of 32 random bytes — openssl rand -base64 32; distinct from the cookie session enckey
}
