package mysql

import (
	"context"
	"database/sql"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Client implements sqldbs.Client for MySQL.
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
		return fmt.Errorf("mysql db: %w", err)
	}
	if _, exists := c.dbs[name]; exists {
		return fmt.Errorf("mysql db: %q already exists", name)
	}

	var dsn string
	if c.conf.DSN != "" {
		dsn = c.conf.DSN
	} else {
		// parseTime and loc are the scan layer's requirement, not a policy
		// choice: time columns must arrive as time.Time in the app's zone.
		// Everything else the connection should say is conf's to state.
		dsn = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s?parseTime=true&loc=%s%s",
			c.conf.User, c.conf.PW,
			c.conf.Host, c.conf.Port,
			dbConf.DB, c.conf.TZ,
			dsnParams(c.conf.Params),
		)
	}

	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return fmt.Errorf("mysql db %q: %w", name, err)
	}
	// ToDo: get these values from conf
	conn.SetConnMaxLifetime(3 * time.Minute)
	conn.SetMaxOpenConns(10)
	conn.SetMaxIdleConns(10)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err = conn.PingContext(ctx); err != nil {
		_ = conn.Close()
		return fmt.Errorf("mysql db %q ping: %w", name, err)
	}

	// Construction includes the schema snapshot: a DB is never handed out
	// without one, which is what lets CachedSchema promise non-nil.
	db := &DB{conn: conn, client: c}
	if _, err := db.RefreshSchema(ctx); err != nil {
		_ = conn.Close()
		return fmt.Errorf("mysql db %q schema: %w", name, err)
	}

	log.Printf("[INFO] mysql db %q initialized (%s)", name, dbConf.DB)
	c.dbs[name] = db
	return nil
}

func (c *Client) DB(name string) (sqldbs.DB, bool) {
	db, ok := c.dbs[name]
	return db, ok
}

func (c *Client) Close() error {
	for name, db := range c.dbs {
		log.Printf("[INFO] closing mysql db %q", name)
		if err := db.conn.Close(); err != nil {
			log.Printf("[ERROR] failed to close mysql db %q: %v", name, err)
		} else {
			log.Printf("[INFO] mysql db %q closed", name)
		}
	}
	return nil
}

// Raw SQL Store

func (c *Client) RawSQLStore(name string) *sqldbs.RawSQLStore {
	return c.stores[name]
}

// LoadRawSQL loads SQL statements from the given FS into a named store.
// Picks .sql (standard) and .mysql (dialect-specific) files.
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
	return "?"
}

func (c *Client) NthPlaceholder(_ int) string {
	return "?"
}

func (c *Client) InPlaceholders(_, cnt int) string {
	placeholders := make([]string, cnt)
	for i := range placeholders {
		placeholders[i] = "?"
	}
	return strings.Join(placeholders, ",")
}

// Identifier Quoting

// QuoteIdentifier quotes an identifier for MySQL, escaping the quote character
// by doubling it. Without that, a name containing a backtick closes the quoting
// early and everything after it is parsed as SQL — identifiers reach here from
// caller-supplied column lists, so this is an injection boundary, not cosmetics.
//
// A dot separates parts of a qualified name, so each part is quoted on its own:
// user.email becomes `user`.`email`, not `user.email`, which would name a single
// column with a dot in it.
func (c *Client) QuoteIdentifier(name string) string {
	parts := strings.Split(name, ".")
	for i, part := range parts {
		parts[i] = "`" + strings.ReplaceAll(part, "`", "``") + "`"
	}
	return strings.Join(parts, ".")
}

// PrepareClients loads MySQL client configs from .sqldb-clients-mysql.json
// and registers them into the provided client map.
func PrepareClients(appRoot string, clients map[string]sqldbs.Client) error {
	confBytes, err := os.ReadFile(filepath.Join(appRoot, "config", ".sqldb-clients-mysql.json"))
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

// dsnParams renders conf Params as DSN query parameters, sorted by key so the
// same conf always produces the same DSN (a connection string that varies per
// boot is one nobody can reason about). Values are written verbatim — see the
// Params doc in clientconf.go.
func dsnParams(params map[string]string) string {
	if len(params) == 0 {
		return ""
	}
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteByte('&')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params[k])
	}
	return b.String()
}
