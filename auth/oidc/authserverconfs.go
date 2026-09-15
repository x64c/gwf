package oidc

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
)

// AuthServerConfs is an auth server's identity providers, keyed identity
// provider id → the name of the registered bearer client → the Provider (the
// IdP client that client initiated with). One identity provider's map is what
// DelegatedExchangeAuthCodeVerifyHandler.Providers takes.
type AuthServerConfs map[string]map[string]*Provider

// LoadAuthServerConfs reads <appRoot>/config/.web-authserver-oidc.json —
// {"<idp id>": {"<client name>": {"issuer": …, "auth_url": …, …}, …}, …} —
// rejecting unknown members, and validates each Provider. A file with no
// identity provider, or an identity provider with no client, fails. Call at
// boot; a file that doesn't load must not serve.
func LoadAuthServerConfs(appRoot string) (AuthServerConfs, error) {
	path := filepath.Join(appRoot, "config", ".web-authserver-oidc.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var confs AuthServerConfs
	if err = json.Unmarshal(b, &confs, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("oidc auth server confs %s: %w", path, err)
	}
	if len(confs) == 0 {
		return nil, fmt.Errorf("oidc auth server confs %s: no identity provider", path)
	}
	for idp, byClient := range confs {
		if len(byClient) == 0 {
			return nil, fmt.Errorf("oidc auth server confs %s: %q has no client", path, idp)
		}
		for name, p := range byClient {
			if p == nil {
				return nil, fmt.Errorf("oidc auth server confs %s: %q/%q is null", path, idp, name)
			}
			if err = p.Validate(); err != nil {
				return nil, fmt.Errorf("oidc auth server confs %s: %q/%q: %w", path, idp, name, err)
			}
		}
	}
	return confs, nil
}
