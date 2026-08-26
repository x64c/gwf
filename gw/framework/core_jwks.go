package framework

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/x64c/gwf/gw/security"
)

func (c *Core) LoadJwksServiceConf() error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".jwks.json")
	confBytes, err := os.ReadFile(confFilePath) // ([]byte, error)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(confBytes, &c.JwksServiceConf); err != nil {
		return err
	}
	return nil
}

// activeKidKVKey returns the KVDB key under which the active JWKS kid is stored.
// Namespaced by AppName so multiple apps sharing a KVDB don't collide.
func (c *Core) activeKidKVKey() string {
	return c.AppName + ":kid"
}

// ActiveKid reads the currently active JWKS key id (kid) from MainKVDB.
// Returns (kid, found, err). found=false indicates the kid hasn't been set yet.
func (c *Core) ActiveKid(ctx context.Context) (string, bool, error) {
	return c.MainKVDB.GetValue(ctx, c.activeKidKVKey())
}

// SetActiveKid stores kid in MainKVDB as the active JWKS key id, with no TTL.
func (c *Core) SetActiveKid(ctx context.Context, kid string) error {
	return c.MainKVDB.SetValuePersistent(ctx, c.activeKidKVKey(), kid)
}

// SignIDToken signs an RS256 ID token with the active JWKS key: `iss`, `sub`,
// `email`, `aud`, `iat`, and `exp` = now + ttl; the header carries the
// active kid. The private key is read from JwksServiceConf.PrivateKeyDir on
// each call. Fails when no active kid is set or its key cannot be loaded.
func (c *Core) SignIDToken(ctx context.Context, iss, sub, email, aud string, ttl time.Duration) (string, error) {
	kid, found, err := c.ActiveKid(ctx)
	if err != nil {
		return "", fmt.Errorf("active kid: %w", err)
	}
	if !found {
		return "", errors.New("active kid not set")
	}
	privateKey, err := security.LoadLocalPrivatePEMKey(security.PrivateKeyPath(c.JwksServiceConf.PrivateKeyDir, kid))
	if err != nil {
		return "", fmt.Errorf("active key %s: %w", kid, err)
	}
	return security.GenerateRSASignedJWTIDToken(iss, sub, email, aud, privateKey, kid, ttl)
}
