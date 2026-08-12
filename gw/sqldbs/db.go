package sqldbs

import "context"

// DB is the common database interface across supported databases.
// Only methods shared by major SQL databases are included.
// For driver-specific features, type-assert to the concrete DB type.
//
// A DB is one database, created by its Client (CreateDB) and handed out by
// name (Client.DB). It embeds Executor: statements run on it directly, each
// call complete on its own. Work that must stand or fall together goes
// through BeginTx.
//
// A DB MUST be safe for concurrent use — one instance serves every request an
// app handles. The exception is SetMainRawSQLStore, which is wiring: a plain
// write, meant for boot, before the DB is shared.
type DB interface {
	Executor

	// Prepare parses a statement once so it can be run repeatedly with different
	// arguments. What comes back belongs to the caller, who must Close it — see
	// PreparedStmt, which holds the obligations that come with owning one.
	Prepare(ctx context.Context, query string) (PreparedStmt, error)

	// Ping checks the database answers. It reports the state at the moment it
	// returns and nothing beyond it: a connection can die between a successful
	// Ping and the next statement, so this is a health signal, not a guarantee to
	// act on.
	Ping(ctx context.Context) error

	// Transaction

	// BeginTx starts a transaction. What comes back must be finished exactly once
	// — see Tx, which carries that obligation and what it costs to ignore.
	//
	// The ctx governs the transaction's whole life, not just this call: an
	// implementation may tie the transaction to it, so cancelling it can end the
	// transaction rather than merely abandoning the attempt to start one.
	BeginTx(ctx context.Context) (Tx, error)

	// Schema Inspection

	// PKColumnOf asks the database for the table's primary key: the column's
	// name, and whether it auto-increments — the property Table declares as
	// PKAutoIncrement, and the one InsertRowAutoIncrementingPK requires.
	//
	// The name is resolved in the database this DB is connected to; a DB cannot
	// answer for another. A table that cannot be found, or one with no primary
	// key, is an error — never an empty name.
	//
	// The answer covers single-column keys. For a primary key spanning several
	// columns it reports one of them, and which one is not specified.
	//
	// ToDo: seed of the DB-owned schema catalog (db.Table(name), built once at
	// boot). The catalog work adds column-list introspection and decides the
	// composite-key answer.
	PKColumnOf(ctx context.Context, table string) (column string, incrementing bool, err error)

	// FetchSchema asks the database what it actually contains: each table the
	// connected database reports, with its columns — names, the dialect's own
	// type spellings, nullability, defaults — and its primary key. Facts, not
	// claims: names and types are recorded as the database states them, not
	// validated against this package's identifier rules, because these are
	// what claims get checked against.
	//
	// Fetch only — the DB holds nothing. What comes back is a snapshot, true
	// at the moment it returns and kept current by nothing; storing it, and
	// fetching again for fresher facts, are the caller's. Meant for the cold
	// path: boot verification, inspection, tooling — never per request.
	//
	// Scope is the connected database; a DB cannot answer for another. What
	// counts as reported within it (views, extra namespaces) is the
	// implementation's own, stated in its docs.
	FetchSchema(ctx context.Context) (*Schema, error)

	// CachedSchema is the held snapshot: fetched when this DB was constructed,
	// replaced only by RefreshSchema. Never nil — constructing a DB includes
	// the fetch, and fails rather than build one without it — and safe for
	// concurrent use.
	//
	// Held means aging: it says what the database contained THEN. Compare
	// against FetchSchema for the live truth, or refresh.
	CachedSchema() *Schema

	// RefreshSchema fetches fresh facts and makes them the held snapshot,
	// returning them. Snapshots already handed out are untouched — a holder
	// keeps the version it read until it asks again.
	RefreshSchema(ctx context.Context) (*Schema, error)

	// Raw SQL Store

	// SetMainRawSQLStore chooses, by name from the Client's stores, the store
	// MainRawSQLStore hands out. It is wiring, not lookup: call it at boot,
	// before the DB is shared — nothing synchronizes it against readers.
	//
	// The signature has no way to report a name that matches no store, so that
	// mistake cannot surface here; it surfaces at the first MainRawSQLStore
	// call instead.
	// ToDo: return an error — boot is where the miswiring happens and an error
	// there has somewhere to go. Interface signature + both impls.
	SetMainRawSQLStore(name string)

	// MainRawSQLStore is the store chosen by SetMainRawSQLStore — the app's
	// main body of named SQL, consulted on most request paths. It MUST NOT
	// return nil: on a DB that was never wired it panics instead.
	MainRawSQLStore() *RawSQLStore
}
