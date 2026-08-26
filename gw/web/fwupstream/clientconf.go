package fwupstream

// ClientConf is the static config for an upstream FW client.
type ClientConf struct {
	Host                       string            `json:"host"`
	ClientID                   string            `json:"client_id"`                 // the downstream's id as a client at the upstream
	RefreshAccessTokenEndpoint string            `json:"refresh_access_token"`      // path after host
	RefreshIdTokenEndpoint     string            `json:"refresh_id_token"`          // path after host
	JwksURL                    string            `json:"jwks_url"`                  // full url
	VerifyAuthCodeEndpoints    map[string]string `json:"verify_external_auth_code"` // path after host, keyed by OAuth provider id

	// User token exchange (the downstream, as a machine client, asking the
	// upstream for a user's bearer): the exchange and revocation endpoints,
	// and the assertion claim that names the user.
	TokenExchangeEndpoint string `json:"token_exchange"` // path after host
	TokenRevokeEndpoint   string `json:"token_revoke"`   // path after host
	UserClaim             string `json:"user_claim"`     // claim carrying the user in the exchange assertion
}
