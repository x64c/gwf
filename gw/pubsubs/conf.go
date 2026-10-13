package pubsubs

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
)

// Conf is what every broker's subscriptions are sized by.
type Conf struct {
	QueueCapacity int `json:"queue_capacity"` // REQUIRED (> 0). Messages each subscription's queue holds, allocated when it subscribes. A subscription whose queue is full when a message comes is ended (EndByFullQueue), so this is the largest burst a subscriber may fall behind by.
}

// LoadConf reads <appRoot>/config/.pubsubs.json, rejecting unknown members, and validates what it read.
func LoadConf(appRoot string) (Conf, error) {
	path := filepath.Join(appRoot, "config", ".pubsubs.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return Conf{}, err
	}
	var conf Conf
	if err = json.Unmarshal(b, &conf, json.RejectUnknownMembers(true)); err != nil {
		return Conf{}, fmt.Errorf("pubsubs conf %s: %w", path, err)
	}
	if err = conf.Validate(); err != nil {
		return Conf{}, fmt.Errorf("pubsubs conf %s: %w", path, err)
	}
	return conf, nil
}

// Validate refuses a conf with a value that is unset or cannot describe what it names.
func (c Conf) Validate() error {
	if c.QueueCapacity <= 0 {
		return fmt.Errorf("queue_capacity must be set (messages > 0): got %d", c.QueueCapacity)
	}
	return nil
}
