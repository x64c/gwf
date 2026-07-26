package sqldbs

import "context"

// DB is the common database interface across supported databases.
// Only methods shared by major SQL databases are included.
// For driver-specific features, type-assert to the concrete DB type.
type DB interface {
	Executor

	// Prepare - Create a reusable prepared statement
	Prepare(ctx context.Context, query string) (PreparedStmt, error)
	// Ping - Check if the connection is alive
	Ping(ctx context.Context) error

	// Transaction

	// BeginTx - Start a transaction
	BeginTx(ctx context.Context) (Tx, error)

	// Schema Inspection

	// PKColumnOf - Fetch the primary key column name and whether it auto-increments
	PKColumnOf(ctx context.Context, table string) (column string, incrementing bool, err error)

	// Raw SQL Store

	// SetMainRawSQLStore - Set primary RawSQLStore by name from Client's stores
	SetMainRawSQLStore(name string)
	// MainRawSQLStore - Primary RawSQLStore — set by SetMainRawSQLStore() beforehand
	MainRawSQLStore() *RawSQLStore
}
