package jwtassert

import (
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// SignerConf is how a downstream proves itself to one upstream as a machine
// client: the key it signs with (by kid and private PEM path), the audience
// the upstream expects, and the assertion lifetime. The downstream's id at
// the upstream comes from its fwupstream client conf, not from here.
type SignerConf struct {
	Kid            string `json:"kid"`
	PrivateKeyPath string `json:"private_key_path"`
	Audience       string `json:"audience"`
	MaxAge         int    `json:"max_age"` // seconds
}

// Validate reports the first missing or non-positive required field.
func (c *SignerConf) Validate() error {
	switch {
	case c.Kid == "":
		return errors.New("jwtassert signer conf: kid required")
	case c.PrivateKeyPath == "":
		return errors.New("jwtassert signer conf: private_key_path required")
	case c.Audience == "":
		return errors.New("jwtassert signer conf: audience required")
	case c.MaxAge <= 0:
		return errors.New("jwtassert signer conf: max_age must be > 0")
	}
	return nil
}

// SignerConfs is a downstream's signers, keyed by the fwupstream client id
// each one signs for.
type SignerConfs map[string]*SignerConf

// LoadSignerConfs reads <appRoot>/config/.fwupstream-jwtassert.json —
// {"<fwupstream client id>": {"kid": …, "private_key_path": …, "audience":
// …, "max_age": …}, …} — rejecting unknown members, and validates each
// entry. Call at boot; a file that doesn't load must not serve.
func LoadSignerConfs(appRoot string) (SignerConfs, error) {
	path := filepath.Join(appRoot, "config", ".fwupstream-jwtassert.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var confs SignerConfs
	if err = json.Unmarshal(b, &confs, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("jwtassert signer confs %s: %w", path, err)
	}
	if len(confs) == 0 {
		return nil, fmt.Errorf("jwtassert signer confs %s: no signer", path)
	}
	for id, c := range confs {
		if c == nil {
			return nil, fmt.Errorf("jwtassert signer confs %s: %q is null", path, id)
		}
		if err = c.Validate(); err != nil {
			return nil, fmt.Errorf("jwtassert signer confs %s: %q: %w", path, id, err)
		}
	}
	return confs, nil
}

// NewSigner builds the Signer for one upstream from its conf and id (the
// downstream's id at that upstream), reading the private key from
// c.PrivateKeyPath.
func NewSigner(c *SignerConf, id string) (*Signer, error) {
	pemBytes, err := os.ReadFile(c.PrivateKeyPath)
	if err != nil {
		return nil, fmt.Errorf("jwtassert signer %s: private key: %w", c.Kid, err)
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM(pemBytes)
	if err != nil {
		return nil, fmt.Errorf("jwtassert signer %s: private key: %w", c.Kid, err)
	}
	return &Signer{
		ID:         id,
		Audience:   c.Audience,
		Kid:        c.Kid,
		PrivateKey: key,
		MaxAge:     time.Duration(c.MaxAge) * time.Second,
	}, nil
}
