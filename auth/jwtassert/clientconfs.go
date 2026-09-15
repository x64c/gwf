package jwtassert

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
)

// ClientConfs is a verifying side's clients, keyed by human name — the map
// NewVerifier takes.
type ClientConfs map[string]*Client

// LoadClientConfs reads <appRoot>/config/.web-authn-jwtassert.json —
// {"<client name>": {"id": …, "audience": …, "public_key_dir": …, "max_age":
// …, "clock_skew": …, "max_body_bytes": …}, …} — rejecting unknown members,
// and validates each entry. A file with no client fails. Call at boot; a file
// that doesn't load must not serve.
func LoadClientConfs(appRoot string) (ClientConfs, error) {
	path := filepath.Join(appRoot, "config", ".web-authn-jwtassert.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var confs ClientConfs
	if err = json.Unmarshal(b, &confs, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("jwtassert client confs %s: %w", path, err)
	}
	if len(confs) == 0 {
		return nil, fmt.Errorf("jwtassert client confs %s: no client", path)
	}
	for name, c := range confs {
		if c == nil {
			return nil, fmt.Errorf("jwtassert client confs %s: %q is null", path, name)
		}
		if err = c.Validate(); err != nil {
			return nil, fmt.Errorf("jwtassert client confs %s: %q: %w", path, name, err)
		}
	}
	return confs, nil
}
