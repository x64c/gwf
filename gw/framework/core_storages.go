package framework

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/storages"
)

func (c *Core) PrepareStorageClients(preparers ...func(string, map[string]storages.Client) error) error {
	c.StorageClients = make(map[string]storages.Client)
	for _, fn := range preparers {
		if err := fn(c.AppRoot, c.StorageClients); err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) PrepareStorages() error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".storages.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return fmt.Errorf("storages: %w", err)
	}
	var clientStoragesConfMap map[string]map[string]jsontext.Value
	if err = json.Unmarshal(confBytes, &clientStoragesConfMap); err != nil {
		return fmt.Errorf("storages: %w", err)
	}
	for clientName, storagesConfMap := range clientStoragesConfMap {
		if clientName == "local" {
			if c.LocalStorages == nil {
				c.LocalStorages = make(map[string]*storages.LocalStorage, len(storagesConfMap))
			}
			for storageName, storageRawConf := range storagesConfMap {
				var storageConf struct {
					Root string `json:"root"`
				}
				if err = json.Unmarshal(storageRawConf, &storageConf); err != nil {
					return fmt.Errorf("storages[local][%s]: %w", storageName, err)
				}
				if storageConf.Root == "" {
					return fmt.Errorf("storages[local][%s]: root is required", storageName)
				}
				c.LocalStorages[storageName] = storages.NewLocalStorage(storageConf.Root)
			}
			continue
		}
		client, ok := c.StorageClients[clientName]
		if !ok {
			return fmt.Errorf("storages[%s]: unknown client", clientName)
		}
		for storageName, storageRawConf := range storagesConfMap {
			if err = client.CreateStorage(storageName, storageRawConf); err != nil {
				return fmt.Errorf("storages[%s][%s]: %w", clientName, storageName, err)
			}
		}
	}
	return nil
}
