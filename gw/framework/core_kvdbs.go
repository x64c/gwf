package framework

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/kvdbs"
)

func (c *Core) PrepareKVDBClients(preparers ...func(string, map[string]kvdbs.Client) error) error {
	c.KVDBClients = make(map[string]kvdbs.Client)
	for _, fn := range preparers {
		if err := fn(c.AppRoot, c.KVDBClients); err != nil {
			return err
		}
	}
	return nil
}

func (c *Core) PrepareKVDatabases() error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".kvdbs.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return fmt.Errorf("kvdbs: %w", err)
	}
	var clientDBsConfMap map[string]map[string]jsontext.Value
	if err = json.Unmarshal(confBytes, &clientDBsConfMap); err != nil {
		return fmt.Errorf("kvdbs: %w", err)
	}
	for clientName, dbsConfMap := range clientDBsConfMap {
		client, ok := c.KVDBClients[clientName]
		if !ok {
			return fmt.Errorf("kvdbs[%s]: unknown client", clientName)
		}
		for dbName, dbRawConf := range dbsConfMap {
			if err = client.PrepareDB(dbName, dbRawConf); err != nil {
				return fmt.Errorf("kvdbs[%s][%s]: %w", clientName, dbName, err)
			}
		}
	}
	return nil
}
