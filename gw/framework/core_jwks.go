package framework

import (
	"context"
	"encoding/json/v2"
	"os"
	"path/filepath"
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
	return c.MainKVDB.Get(ctx, c.activeKidKVKey())
}

// SetActiveKid stores kid in MainKVDB as the active JWKS key id, with no TTL.
func (c *Core) SetActiveKid(ctx context.Context, kid string) error {
	return c.MainKVDB.Set(ctx, c.activeKidKVKey(), kid, 0)
}
