package redis

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	lowimpl "github.com/redis/go-redis/v9"
	"github.com/x64c/gwf/gw/clock"
	"github.com/x64c/gwf/gw/kvdbs"
)

// ClientConf holds shared Redis server connection config.
type ClientConf struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	PW   string `json:"pw"`

	// InitTimeoutSecs is the deadline for EACH database's whole
	// initialization in PrepareDB: connect + ping. REQUIRED (seconds > 0). A
	// tolerance judgment only the deployment can make — a server on the same
	// host may want fail-fast, one across a link or resuming from cold may
	// need far more.
	InitTimeoutSecs int `json:"init_timeout_secs"`
}

// DBConf holds per-database config.
type DBConf struct {
	DB int `json:"db"` // Redis db number (0–15)
}

// Client implements kvdbs.Client for Redis.
// One Client = one server (host + port + credentials).
type Client struct {
	conf ClientConf
	dbs  map[string]*DB
}

func NewClient(conf ClientConf) (*Client, error) {
	if conf.InitTimeoutSecs <= 0 {
		return nil, fmt.Errorf("redis: init_timeout_secs must be set (seconds > 0): got %d — the deployment states its tolerance", conf.InitTimeoutSecs)
	}
	return &Client{
		conf: conf,
		dbs:  make(map[string]*DB),
	}, nil
}

func (c *Client) PrepareDB(name string, rawConf jsontext.Value) error {
	var dbConf DBConf
	if err := json.Unmarshal(rawConf, &dbConf); err != nil {
		return fmt.Errorf("redis db: %w", err)
	}
	if _, exists := c.dbs[name]; exists {
		return fmt.Errorf("redis db: %q already exists", name)
	}
	internal := lowimpl.NewClient(&lowimpl.Options{
		Addr:     fmt.Sprintf("%s:%d", c.conf.Host, c.conf.Port),
		Password: c.conf.PW,
		DB:       dbConf.DB,
	})

	// Construction includes reaching the server: a DB is never handed out
	// without having answered once, so a misconfigured or unreachable store
	// fails here rather than at the first operation.
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.conf.InitTimeoutSecs)*time.Second)
	defer cancel()
	if err := internal.Ping(ctx).Err(); err != nil {
		_ = internal.Close()
		return fmt.Errorf("redis db %q ping: %w", name, err)
	}

	log.Printf("[INFO] redis db %q initialized (db %d)", name, dbConf.DB)
	c.dbs[name] = &DB{
		internal: internal,
		clock:    clock.New(TimePrecision),
	}
	return nil
}

func (c *Client) DB(name string) (kvdbs.DB, bool) {
	db, ok := c.dbs[name]
	return db, ok
}

func (c *Client) Close() error {
	for name, db := range c.dbs {
		if err := db.internal.Close(); err != nil {
			log.Printf("[ERROR] failed to close redis db %q: %v", name, err)
		}
	}
	return nil
}

// PrepareClients loads Redis client configs from .kvdb-clients-redis.json
// and registers them into the provided client map.
func PrepareClients(appRoot string, clients map[string]kvdbs.Client) error {
	confBytes, err := os.ReadFile(filepath.Join(appRoot, "config", ".kvdb-clients-redis.json"))
	if err != nil {
		return err
	}
	var confs map[string]ClientConf
	if err = json.Unmarshal(confBytes, &confs); err != nil {
		return err
	}
	for name, conf := range confs {
		client, err := NewClient(conf)
		if err != nil {
			return fmt.Errorf("redis client %q: %w", name, err)
		}
		clients[name] = client
	}
	return nil
}
