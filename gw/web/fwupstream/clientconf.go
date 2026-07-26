package fwupstream

// ClientConf is the static config for an upstream FW client.
type ClientConf struct {
	Host                       string            `json:"host"`
	ClientID                   string            `json:"client_id"`                 // ID of this app as a client to the upstream FW app
	RefreshAccessTokenEndpoint string            `json:"refresh_access_token"`      // path after host
	RefreshIdTokenEndpoint     string            `json:"refresh_id_token"`          // path after host
	JwksURL                    string            `json:"jwks_url"`                  // full url
	VerifyAuthCodeEndpoints    map[string]string `json:"verify_external_auth_code"` // path after host, keyed by OAuth provider id
}
