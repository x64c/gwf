package framework

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"

	"github.com/x64c/gwf/gw/sqldbs"
)

func (c *Core) PrepareSQLDBClients(preparers ...func(string, map[string]sqldbs.Client) error) error {
	c.SQLDBClients = make(map[string]sqldbs.Client)
	for _, fn := range preparers {
		if err := fn(c.AppRoot, c.SQLDBClients); err != nil {
			return err
		}
	}
	for fsName, sqlFS := range c.RawSQLFSMap {
		for clientName, client := range c.SQLDBClients {
			if err := client.LoadRawSQL(fsName, sqlFS); err != nil {
				return fmt.Errorf("sqldb client %q: load %q: %w", clientName, fsName, err)
			}
		}
	}
	return nil
}

func (c *Core) PrepareSQLDatabases() error {
	confFilePath := filepath.Join(c.AppRoot, "config", ".sqldbs.json")
	confBytes, err := os.ReadFile(confFilePath)
	if err != nil {
		return fmt.Errorf("sqldbs: %w", err)
	}
	var clientDBsConfMap map[string]map[string]jsontext.Value
	if err = json.Unmarshal(confBytes, &clientDBsConfMap); err != nil {
		return fmt.Errorf("sqldbs: %w", err)
	}
	for clientName, dbsConfMap := range clientDBsConfMap {
		client, ok := c.SQLDBClients[clientName]
		if !ok {
			return fmt.Errorf("sqldbs[%s]: unknown client", clientName)
		}
		for dbName, dbRawConf := range dbsConfMap {
			if err = client.CreateDB(dbName, dbRawConf); err != nil {
				return fmt.Errorf("sqldbs[%s][%s]: %w", clientName, dbName, err)
			}
		}
	}
	return nil
}
