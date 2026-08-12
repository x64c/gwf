package sqldbs

import "context"

// Executor is the shared interface between DB and Tx for executing SQL.
// Use this as the parameter type when the caller should work with either.
//
// Its methods are one tool and two kinds, laid out in that order. Client
// leads: the dialect a statement is written WITH, consulted before anything
// runs. Then the raw methods: they run SQL the caller wrote, and they are the
// foundation — every built method funnels down to Exec, QueryRowRaw or
// QueryRowsRaw. Then the builders: handed a table, columns and a condition,
// the implementation writes the SQL itself.
//
// Building lives here rather than in a layer above because an implementation IS
// a dialect — it quotes through its own Client and writes its own placeholders,
// so one operation comes out differently in each. Implementations resembling
// each other is therefore expected; what is worth watching is shared logic
// drifting apart between them.
//
// An implementation may also step outside SQL where its backend offers
// something better suited — a bulk-load stream in place of a statement, say.
// Properties this interface does not state can differ with that choice: how
// much a single call carries, how a partial failure surfaces. Each method says
// what every implementation must guarantee; past that, do not infer from
// whichever one happens to be in front of you.
type Executor interface {
	// Foundation tool

	// Client is the connection's dialect — identifier quoting and bind
	// placeholders: what a statement is written WITH, consulted before
	// anything runs. Callers building SQL by hand need it; so does anything
	// turning a Cond into a WHERE clause.
	Client() Client

	// Raw — the caller writes the statement.

	// Exec runs one statement written by the caller and reports how many rows it
	// changed.
	//
	// This is the lowest level the interface offers: no statement is built, no
	// verb is checked, nothing is scanned. Every other method here is a narrower
	// form of it, and anything they cannot express belongs here.
	//
	// Exec hands back no result set. A statement that produces one — a SELECT, or
	// an INSERT ... RETURNING — still runs and its changes still persist, but the
	// rows it selected cannot be reached through Exec. Read them with QueryRowRaw
	// or QueryRowsRaw.
	//
	// The count is the rows the statement changed, and is meaningful for INSERT,
	// UPDATE and DELETE. For a statement that changes nothing — DDL, or a SELECT
	// run for its effect — it is NOT specified: implementations report whatever
	// their backend says the statement did, and backends disagree.
	Exec(ctx context.Context, query string, args ...any) (int64, error)

	// Query — any row-returning statement, no verb guard.
	// Use for SELECT, INSERT/UPDATE/DELETE ... RETURNING, CTEs, etc.
	// Caller owns the SQL correctness.
	// You can use the verb-guarded Select methods below for strict SELECT guarding.

	// QueryRowRaw runs a caller-written statement and hands back its first row.
	//
	// It returns no error. EVERY failure — malformed SQL, a dead connection, no
	// matching row, a value the destination cannot hold — arrives from Scan, so a
	// caller that ignores Scan's error learns nothing at all. A missing row is
	// not special-cased: it reaches Scan as ErrNoRows.
	//
	// Scan must be called exactly once, even when the values are not wanted. It
	// is what finishes the query and frees the connection the row is holding.
	QueryRowRaw(ctx context.Context, query string, args ...any) Row

	// QueryRowsRaw runs a caller-written statement and hands back every row.
	//
	// Unlike QueryRowRaw it reports a failure to start the query straight away.
	// A failure part-way through iteration does not stop the loop by itself and
	// is not returned by Next — it surfaces from Rows.Err, which the caller must
	// check afterwards, because a loop cut short otherwise looks exactly like one
	// that finished.
	//
	// The caller owns the Rows and must Close it; that is what frees the
	// connection.
	QueryRowsRaw(ctx context.Context, query string, args ...any) (Rows, error)

	// Verb-guarded — still caller-written SQL, with a first-word check added.

	// SelectRowRaw and SelectRowsRaw run a statement the caller wrote, requiring
	// it to begin with SELECT. They are QueryRowRaw and QueryRowsRaw with that one
	// check added, and differ in nothing else.
	//
	// The check is a prefix test on the first word. It catches a mutation handed
	// to a reading method by mistake. It does NOT make the statement read-only —
	// nothing after a semicolon is examined — and it turns away reads that do not
	// open with the word, a WITH ... SELECT among them. Use the Query pair for
	// those.
	SelectRowRaw(ctx context.Context, query string, args ...any) (Row, error)
	SelectRowsRaw(ctx context.Context, query string, args ...any) (Rows, error)

	// InsertRowsRaw runs an INSERT the caller wrote and reports how many rows it
	// inserted, requiring the statement to begin with INSERT — the same prefix
	// test as the Select pair, with the same limits.
	//
	// Rows is in the name because the count is what comes back; the statement
	// itself may insert one row or many.
	InsertRowsRaw(ctx context.Context, query string, args ...any) (int64, error)

	// UpdateRowsRaw runs an UPDATE the caller wrote and reports how many rows
	// changed, requiring the statement to begin with UPDATE — the same prefix
	// test as the Select pair, with the same limits.
	UpdateRowsRaw(ctx context.Context, query string, args ...any) (int64, error)

	// DeleteRowsRaw runs a DELETE the caller wrote and reports how many rows went,
	// requiring the statement to begin with DELETE — the same prefix test as the
	// Select pair, with the same limits.
	DeleteRowsRaw(ctx context.Context, query string, args ...any) (int64, error)

	// Built — handed a Table, columns and a condition, the implementation
	// writes the statement itself.

	// Select

	// SelectRow reads one row by primary key: pkValue carries one value per
	// key column, and colNames picks the columns to read — empty means all of
	// them. Both are checked against the table before anything runs: a
	// pkValue of the wrong width, or a name the table does not declare, is
	// refused rather than reaching SQL.
	//
	// The returned error reports only what could be judged before running. The
	// query's own outcome arrives from Row.Scan, as it does wherever a Row is
	// returned.
	SelectRow(ctx context.Context, table *Table, pkValue PK, colNames ...string) (Row, error)

	// SelectRows reads every row matching where, or the whole table when where is
	// nil. colNames picks the columns to read — empty means all, and a name the
	// table does not declare is refused.
	//
	// The condition is structured rather than text: the identifiers in it are
	// validated columns and its placeholders are translated to the dialect, so
	// nothing the caller supplies reaches the statement as SQL.
	SelectRows(ctx context.Context, table *Table, where Cond, colNames ...string) (Rows, error)

	// Insert (INSERT INTO table (columns) VALUES (values) ...)
	// Rules for implementers:
	//   - Empty columns MUST return an error (programming error; structure is required).
	//   - For InsertRows with valid columns but empty rowValues, MUST return (0, nil) as a no-op
	//     (zero data is a valid "nothing to insert" case).

	// InsertRow inserts one row. columns carries what values fills — the two
	// are parallel, empty columns is an error, and every name must be one the
	// table declares. It returns no count: the statement inserts its one row
	// or fails, so a count could only repeat what the error already says.
	InsertRow(ctx context.Context, table *Table, columns []string, values []any) error

	// InsertRows inserts many rows in one call and reports how many landed.
	//
	// How many rows a single call can carry is bounded, and the bound belongs to
	// the implementation rather than to this interface: one may build a single
	// statement, limited by its size or its bind-parameter count, while another
	// streams and is limited by neither. The same call can therefore succeed
	// against one and fail against another purely on row count, so a caller
	// holding a large set should chunk it rather than assume a ceiling.
	//
	// columns must name only columns the table declares, as everywhere.
	InsertRows(ctx context.Context, table *Table, columns []string, rowValues [][]any) (int64, error)

	// InsertRowAutoIncrementingPK inserts one row WITHOUT its primary key and
	// returns the value the database generated for that key.
	//
	// It promises the generated primary key and nothing else — it is not a
	// general "return a column after writing".
	//
	// Implementers MUST reject, before executing:
	//   - a table that does not declare an auto-increment key (PKAutoIncrement)
	//   - empty columns (a programming error, as elsewhere)
	//   - the key column present in columns — the database assigns that value,
	//     so supplying it contradicts the call. Use InsertRow instead.
	//
	// A declaration is not the schema. A generated key is never 0, so an
	// implementation that cannot otherwise tell that the declared
	// auto-increment does not hold in the database MUST return an error rather
	// than report 0 as if it were a key.
	//
	// An error does NOT prove the row is absent. Reading the generated key is a
	// step that runs after the row is written, so a failure there — including a
	// broken precondition that only becomes visible then — can leave the insert
	// standing. Callers who need all-or-nothing must call this inside a Tx.
	//
	// Nothing about the key sequence is promised beyond the value returned. A
	// rollback undoes the row; whether it also returns the key that row consumed
	// is not guaranteed either way, so callers MUST NOT assume keys are
	// consecutive or gap-free.
	InsertRowAutoIncrementingPK(ctx context.Context, table *Table, columns []string, values []any) (int64, error)

	// Update (UPDATE table SET column = value, ... WHERE ...)
	// Empty columns ends in error (no panic).
	// Implementer's choice: guard upfront for early error,
	// or let the DBMS return a SQL error (saves the guard but costs a round trip).

	// UpdateRow updates the row with that key and reports how many it matched —
	// 0 meaning there is no such row, which is the only way to learn that.
	//
	// pkValue carries one value per key column. columns and values are
	// parallel: the nth value is written to the nth column; empty columns is
	// an error, and every name must be one the table declares. Neither slice
	// is modified.
	UpdateRow(ctx context.Context, table *Table, pkValue PK, columns []string, values []any) (int64, error)

	// UpdateRows updates every row matching where, and reports how many changed.
	//
	// A nil where updates the WHOLE TABLE. That is the SQL meaning of an absent
	// WHERE and this does not second-guess it, so a condition that is nil by
	// accident is indistinguishable from one that is nil on purpose.
	//
	// columns must name only columns the table declares, as everywhere.
	UpdateRows(ctx context.Context, table *Table, columns []string, values []any, where Cond) (int64, error)

	// Delete (DELETE FROM table WHERE ...)

	// DeleteRow deletes the row with that key — pkValue carries one value per
	// key column — and reports how many it matched, 0 meaning there was no
	// such row, which is the only way to learn that. The key columns come from
	// the table, so no caller can point this at a non-key column.
	DeleteRow(ctx context.Context, table *Table, pkValue PK) (int64, error)

	// DeleteRows deletes every row matching where, and reports how many went.
	//
	// A nil where EMPTIES THE TABLE. That is the SQL meaning of an absent WHERE
	// and this does not second-guess it: a condition that came out nil by
	// accident looks exactly like one meant that way, and the rows are gone
	// either way.
	DeleteRows(ctx context.Context, table *Table, where Cond) (int64, error)
}
