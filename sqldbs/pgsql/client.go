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

// NewClient validates the conf and constructs the client. ALL pool
// misconfiguration — a missing block, a missing field, an incoherent value, a
// DSN carrying pool_* beside the pool block — is a construction (= boot)
// error.
func NewClient(conf ClientConf) (*Client, error) {
	if err := conf.Pool.validate(); err != nil {
		return nil, err
	}
	if err := validateDSNCarriesNoPoolParams(conf.DSN); err != nil {
		return nil, err
	}
	if conf.InitTimeoutSecs <= 0 {
		return nil, fmt.Errorf("pgsql: init_timeout_secs must be set (seconds > 0): got %d — the framework used to hardcode 5 seconds; the deployment states its tolerance", conf.InitTimeoutSecs)
	}
	return &Client{
		conf:   conf,
		stores: make(map[string]*sqldbs.RawSQLStore),
		dbs:    make(map[string]*DB),
	}, nil
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
	// Pool tuning comes from the conf's REQUIRED pool block — the single
	// source, validated at NewClient. The DSN cannot carry pool_* (refused at
	// construction), so nothing here overwrites a stated value.
	config.MaxConns = *c.conf.Pool.MaxConns
	config.MinConns = *c.conf.Pool.MinConns
	config.MaxConnLifetime = time.Duration(*c.conf.Pool.ConnMaxLifetimeSecs) * time.Second
	config.MaxConnIdleTime = time.Duration(*c.conf.Pool.ConnMaxIdleTimeSecs) * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.conf.InitTimeoutSecs)*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return fmt.Errorf("pgsql db %q: failed to connect pgx pool: %w", name, err)
	}
	if err = pool.Ping(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("pgsql db %q ping: %w", name, err)
	}

	// Construction includes the schema snapshot: a DB is never handed out
	// without one, which is what lets CachedSchema promise non-nil.
	db := &DB{pool: pool, client: c}
	if _, err := db.RefreshSchema(ctx); err != nil {
		pool.Close()
		return fmt.Errorf("pgsql db %q schema: %w", name, err)
	}

	log.Printf("[INFO] pgsql db %q initialized (%s)", name, dbConf.DB)
	c.dbs[name] = db
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

// QuoteIdentifier quotes an identifier for PostgreSQL, escaping the quote
// character by doubling it. Without that, a name containing `"` closes the
// quoting early and everything after it is parsed as SQL — identifiers reach
// here from caller-supplied column lists, so this is an injection boundary,
// not cosmetics.
//
// A dot separates parts of a qualified name, so each part is quoted on its own:
// user.email becomes "user"."email", not "user.email", which would name a single
// column with a dot in it.
func (c *Client) QuoteIdentifier(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = `"` + strings.ReplaceAll(part, `"`, `""`) + `"`
	}
	return strings.Join(parts, ".")
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
		client, err := NewClient(conf)
		if err != nil {
			return fmt.Errorf("pgsql client %q: %w", name, err)
		}
		clients[name] = client
	}
	return nil
}
