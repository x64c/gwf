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
	c.AddService(c.UDSService)
	return nil
}
