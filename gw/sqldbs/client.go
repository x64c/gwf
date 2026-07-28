package sqldbs

import (
	"encoding/json/jsontext"
	"io/fs"
)

// Client is the common SQL database client interface.
// Holds connection credentials and manages named databases and raw SQL stores.
type Client interface {
	// CreateDB - Create a named database from config
	CreateDB(name string, conf jsontext.Value) error
	// DB - Get a named database
	DB(name string) (DB, bool)
	// Close - Close all connections
	Close() error

	// Raw SQL Store

	// RawSQLStore - Get a named RawSQLStore
	RawSQLStore(name string) *RawSQLStore
	// LoadRawSQL - Load SQL files from an fs.FS into a named store
	LoadRawSQL(name string, sqlFS fs.FS) error

	// Placeholder — DBMS-specific binding syntax

	// FirstPlaceholder - First bind placeholder (e.g. MySQL: "?", PostgreSQL: "$1")
	FirstPlaceholder() string
	// NthPlaceholder - Nth bind placeholder (e.g. MySQL: "?", PostgreSQL: "$N")
	NthPlaceholder(n int) string
	// InPlaceholders - Comma-separated placeholders for IN clause (e.g. MySQL: "?, ?, ?", PostgreSQL: "$7, $8, $9")
	InPlaceholders(start, cnt int) string

	// Identifier Quoting — DBMS-specific identifier quoting

	// QuoteIdentifier - Quote an identifier for the dialect (e.g. MySQL: `name`, PostgreSQL: "name").
	// Identifiers are case-sensitive — the exact casing provided is preserved.
	// Used by structured CRUD methods (InsertRow, UpdateRow, etc.) to quote column/table names.
	//
	// MUST escape the dialect's quote character (SQL escapes it by doubling).
	// Identifiers arrive here from caller-supplied column lists, so an
	// implementation that merely concatenates lets a name containing the quote
	// character close the quoting and inject SQL. Escaping cannot break a
	// legitimate identifier: a name that contains no quote is unchanged.
	QuoteIdentifier(name string) string
}
