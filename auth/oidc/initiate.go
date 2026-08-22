package oidc

import (
	"net/url"
	"strings"

	"github.com/x64c/gwf/gw/authn"
)

// AuthCodeURL builds the authorization URL for the flow a ticket opens:
// state and nonce as parameters, the PKCE verifier as its S256 challenge,
// the provider's scopes and extra parameters. redirectURI is where the
// provider sends the code — in a split relying party it belongs to the
// initiating app, and the verify half must receive the same value.
func (p *Provider) AuthCodeURL(t authn.FlowTicket, redirectURI string) string {
	params := url.Values{}
	for key, value := range p.ExtraAuthParams {
		// extras first: the standard parameters set below win on collision
		params.Set(key, value)
	}
	params.Set("response_type", "code")
	params.Set("client_id", p.ClientID)
	params.Set("redirect_uri", redirectURI)
	params.Set("state", t.State)
	params.Set("nonce", t.Nonce)
	params.Set("scope", strings.Join(p.Scopes, " "))
	params.Set("code_challenge", t.PKCEChallengeS256())
	params.Set("code_challenge_method", "S256")
	return p.AuthURL + "?" + params.Encode()
}
