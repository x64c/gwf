package cors

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
)

// Confs is a deployment's CORS settings, config/.web-cors.json: each access control key's Conf. The file's top-level
// keys are the access control keys.
type Confs map[string]Conf

// LoadConfs reads <appRoot>/config/.web-cors.json, rejecting unknown members, and validates what it read.
func LoadConfs(appRoot string) (Confs, error) {
	path := filepath.Join(appRoot, "config", ".web-cors.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var confs Confs
	if err = json.Unmarshal(b, &confs, json.RejectUnknownMembers(true)); err != nil {
		return nil, fmt.Errorf("cors conf %s: %w", path, err)
	}
	if err = confs.Validate(); err != nil {
		return nil, fmt.Errorf("cors conf %s: %w", path, err)
	}
	return confs, nil
}

// Validate refuses a file that is not an object, an empty key, and any Conf that Conf.Validate refuses.
func (cs Confs) Validate() error {
	if cs == nil {
		return fmt.Errorf("want an object of access control keys")
	}
	for key, c := range cs {
		if key == "" {
			return fmt.Errorf("an empty access control key")
		}
		if err := c.Validate(); err != nil {
			return fmt.Errorf("%q: %w", key, err)
		}
	}
	return nil
}

// Conf returns the Conf of an access control key; a key the file does not have is an error naming it.
func (cs Confs) Conf(key string) (Conf, error) {
	c, ok := cs[key]
	if !ok {
		return Conf{}, fmt.Errorf("cors: access control key %q is not in .web-cors.json", key)
	}
	return c, nil
}
