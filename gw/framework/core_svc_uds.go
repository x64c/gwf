package framework

import (
	"encoding/json/v2"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/uds"
)

func (c *Core) PrepareUDSService(cmdStore *uds.CommandStore) error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".uds.json")
	confBytes, err := os.ReadFile(confFilePath) // ([]byte, error)
	if err != nil {
		return err
	}
	conf := uds.Conf{}
	if err = json.Unmarshal(confBytes, &conf); err != nil {
		return err
	}
	c.UDSService = uds.NewService(conf, cmdStore)
	// The admin socket depends on no service. Its commands reach into others to
	// report on them, which is observation, not dependence: forcing an edge
	// there would order it as a dependent of everything it inspects, so it
	// would start last and terminate first — losing the observer exactly when a
	// hanging boot or a slow drain is the thing worth watching.
	if _, err = c.RegisterService(c.UDSService); err != nil {
		return err
	}
	return nil
}
