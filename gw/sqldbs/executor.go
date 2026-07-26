package sqldbs

import "context"

// Executor is the shared interface between DB and Tx for executing SQL.
// Use this as the parameter type when the caller should work with either.
type Executor interface {
	Exec(ctx context.Context, query string, args ...any) (Result, error)
	Client() Client

	// Query — any row-returning statement, no verb guard.
	// Use for SELECT, INSERT/UPDATE/DELETE ... RETURNING (PostgreSQL), CTEs, etc.
	// Caller owns the SQL correctness.
	// You can use Select methods below for strict SELECT guarding.

	QueryRowRaw(ctx context.Context, query string, args ...any) Row
	QueryRowsRaw(ctx context.Context, query string, args ...any) (Rows, error)

	// Select (SELECT columns FROM table ...) — strictly SELECT.
	// Structured: framework builds the SELECT SQL; empty columns → error.
	// Raw: caller provides SQL; guarded to start with SELECT.

	// SelectRow — fetch a single row by primary key.
	// Returns:
	//   Row interface: has `.Scan(dest ...any) error` method
	//   error: invocation-time of SelectRow()
	SelectRow(ctx context.Context, table string, pkColumn string, id any, columns []string) (Row, error)
	SelectRows(ctx context.Context, table string, columns []string, where Cond) (Rows, error)
	SelectRowRaw(ctx context.Context, query string, args ...any) (Row, error)
	SelectRowsRaw(ctx context.Context, query string, args ...any) (Rows, error)

	// Insert (INSERT INTO table (columns) VALUES (values) ...)
	// Rules for implementers:
	//   - Empty columns MUST return an error (programming error; structure is required).
	//   - For InsertRows with valid columns but empty rowValues, MUST return (0, nil) as a no-op
	//     (zero data is a valid "nothing to insert" case).

	InsertRow(ctx context.Context, table string, columns []string, values []any) (Result, error)
	InsertRows(ctx context.Context, table string, columns []string, rowValues [][]any) (int64, error)
	InsertRowsRaw(ctx context.Context, query string, args ...any) (Result, error)

	// Update (UPDATE table SET column = value, ... WHERE ...)
	// Empty columns ends in error (no panic).
	// Implementer's choice: guard upfront for early error,
	// or let the DBMS return a SQL error (saves the guard but costs a round trip).

	UpdateRow(ctx context.Context, table string, pkColumn string, id any, columns []string, values []any) (Result, error)
	UpdateRows(ctx context.Context, table string, columns []string, values []any, where Cond) (int64, error)
	UpdateRowsRaw(ctx context.Context, query string, args ...any) (Result, error)

	// Delete (DELETE FROM table WHERE ...)

	DeleteRow(ctx context.Context, table string, pkColumn string, id any) (Result, error)
	DeleteRows(ctx context.Context, table string, where Cond) (int64, error)
	DeleteRowsRaw(ctx context.Context, query string, args ...any) (Result, error)
}
