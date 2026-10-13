package redis

import (
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// BrokerConf is where the broker's Redis is. It has no database number: Redis pub/sub channels are not kept per
// database — a publish on one is heard by subscribers on every other.
type BrokerConf struct {
	Host string `json:"host"` // REQUIRED
	Port int    `json:"port"` // REQUIRED (1–65535)
	PW   string `json:"pw"`

	// InitTimeoutSecs is the deadline for Start's connecting and first checks. REQUIRED (seconds > 0): a server on the
	// same host may want fail-fast, one across a link or resuming from cold may need more.
	InitTimeoutSecs int `json:"init_timeout_secs"`
}

// LoadBrokerConf reads <appRoot>/config/.pubsub-broker-redis.json — one object — rejecting unknown members, and
// validates what it read.
func LoadBrokerConf(appRoot string) (BrokerConf, error) {
	path := filepath.Join(appRoot, "config", ".pubsub-broker-redis.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return BrokerConf{}, err
	}
	var conf BrokerConf
	if err = json.Unmarshal(b, &conf, json.RejectUnknownMembers(true)); err != nil {
		return BrokerConf{}, fmt.Errorf("redis broker conf %s: %w", path, err)
	}
	if err = conf.Validate(); err != nil {
		return BrokerConf{}, fmt.Errorf("redis broker conf %s: %w", path, err)
	}
	return conf, nil
}

// Validate refuses a conf with a value that is unset or cannot describe what it names.
func (c BrokerConf) Validate() error {
	if c.Host == "" {
		return fmt.Errorf("host must be set")
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be 1–65535: got %d", c.Port)
	}
	if c.InitTimeoutSecs <= 0 {
		return fmt.Errorf("init_timeout_secs must be set (seconds > 0): got %d", c.InitTimeoutSecs)
	}
	return nil
}

func (c BrokerConf) addr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

func (c BrokerConf) initTimeout() time.Duration {
	return time.Duration(c.InitTimeoutSecs) * time.Second
}
