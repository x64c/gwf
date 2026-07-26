package pgsql

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Client implements sqldbs.Client for PostgreSQL.
// One Client = one server (host + port + credentials).
type Client struct {
	conf   ClientConf
	stores map[string]*sqldbs.RawSQLStore
	dbs    map[string]*DB
}

func NewClient(conf ClientConf) *Client {
	return &Client{
		conf:   conf,
		stores: make(map[string]*sqldbs.RawSQLStore),
		dbs:    make(map[string]*DB),
	}
}

func (c *Client) CreateDB(name string, rawConf jsontext.Value) error {
	var dbConf DBConf
	if err := json.Unmarshal(rawConf, &dbConf); err != nil {
		return fmt.Errorf("pgsql db: %w", err)
	}
	if _, exists := c.dbs[name]; exists {
		return fmt.Errorf("pgsql db: %q already exists", name)
	}

	var dsn string
	if c.conf.DSN != "" {
		dsn = c.conf.DSN
	} else {
		// NOTE: sslmode=disable is often used for local dev, adjust as needed.
		// NOTE: PostgreSQL natively allows multiple statements in a single query string.
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable TimeZone=%s",
			c.conf.Host, c.conf.Port,
			c.conf.User, c.conf.PW,
			dbConf.DB, c.conf.TZ,
		)
	}

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return fmt.Errorf("pgsql db %q: failed to parse pgx config: %w", name, err)
	}
	// Pool tuning _ ToDo: get these values from conf
	config.MaxConns = 10
	config.MinConns = 2
	config.MaxConnLifetime = 3 * time.Minute

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pgsql db %q: failed to connect pgx pool: %w", name, err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("pgsql db %q ping: %w", name, err)
	}

	log.Printf("[INFO] pgsql db %q initialized (%s)", name, dbConf.DB)
	c.dbs[name] = &DB{pool: pool, client: c}
	return nil
}

func (c *Client) DB(name string) (sqldbs.DB, bool) {
	db, ok := c.dbs[name]
	return db, ok
}

func (c *Client) Close() error {
	for name, db := range c.dbs {
		log.Printf("[INFO] closing pgsql db %q", name)
		db.pool.Close()
		log.Printf("[INFO] pgsql db %q closed", name)
	}
	return nil
}

// Raw SQL Store

func (c *Client) RawSQLStore(name string) *sqldbs.RawSQLStore {
	return c.stores[name]
}

// LoadRawSQL loads SQL statements from the given FS into a named store.
// Picks .sql (standard) and .pgsql (dialect-specific) files.
func (c *Client) LoadRawSQL(name string, sqlFS fs.FS) error {
	store := sqldbs.NewRawSQLStore()
	if err := loadRawStmtsToStore(store, sqlFS); err != nil {
		return err
	}
	c.stores[name] = store
	return nil
}

// Placeholder

func (c *Client) FirstPlaceholder() string {
	return "$1"
}

func (c *Client) NthPlaceholder(n int) string {
	return fmt.Sprintf("$%d", n)
}

func (c *Client) InPlaceholders(start, cnt int) string {
	placeholders := make([]string, cnt)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(placeholders, ",")
}

// Identifier Quoting

func (c *Client) QuoteIdentifier(name string) string {
	return `"` + name + `"`
}

// PrepareClients loads PostgreSQL client configs from .sqldb-clients-pgsql.json
// and registers them into the provided client map.
func PrepareClients(appRoot string, clients map[string]sqldbs.Client) error {
	confBytes, err := os.ReadFile(filepath.Join(appRoot, "config", ".sqldb-clients-pgsql.json"))
	if err != nil {
		return err
	}
	var confs map[string]ClientConf
	if err = json.Unmarshal(confBytes, &confs); err != nil {
		return err
	}
	for name, conf := range confs {
		clients[name] = NewClient(conf)
	}
	return nil
}
