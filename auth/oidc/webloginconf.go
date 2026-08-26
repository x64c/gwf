package oidc

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WebLoginConf is one identity provider's browser login on the browser-facing
// side (redirect out to the provider, callback in): the Provider the initiate
// and verify halves run against, and the redirect URI the provider returns
// the browser to (the app's callback endpoint).
type WebLoginConf struct {
	Provider    Provider `json:"provider"`
	RedirectURI string   `json:"redirect_uri"`
}

// Validate reports the first missing required field.
func (c *WebLoginConf) Validate() error {
	if err := c.Provider.Validate(); err != nil {
		return err
	}
	if c.RedirectURI == "" {
		return errors.New("oidc web login conf: redirect_uri required")
	}
	return nil
}

// WebLoginConfs is an app's browser logins, keyed by the identity provider
// id the app chose (the key its routes select a login by).
type WebLoginConfs map[string]*WebLoginConf

// LoadWebLoginConfs reads <appRoot>/config/.web-login-oidc.json — {"<idp
// id>": {"provider": {…}, "redirect_uri": "…"}, …} — rejecting unknown
// members, and validates each entry. Call at boot; a file that doesn't load
// must not serve.
func LoadWebLoginConfs(appRoot string) (WebLoginConfs, error) {
	path := filepath.Join(appRoot, "config", ".web-login-oidc.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var confs WebLoginConfs
	if err = json.Unmarshal(b, &confs, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("oidc web login confs %s: %w", path, err)
	}
	if len(confs) == 0 {
		return nil, fmt.Errorf("oidc web login confs %s: no identity provider", path)
	}
	for id, c := range confs {
		if c == nil {
			return nil, fmt.Errorf("oidc web login confs %s: %q is null", path, id)
		}
		if err = c.Validate(); err != nil {
			return nil, fmt.Errorf("oidc web login confs %s: %q: %w", path, id, err)
		}
	}
	return confs, nil
}
