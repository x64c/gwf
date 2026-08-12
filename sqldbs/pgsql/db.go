package pgsql

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.DB implements sqldbs.DB interface
var _ sqldbs.DB = (*DB)(nil)

type DB struct {
	pool      *pgxpool.Pool
	client    *Client
	mainStore *sqldbs.RawSQLStore
	schema    atomic.Pointer[sqldbs.Schema] // held snapshot — see CachedSchema
}

// Core

func (d *DB) Client() sqldbs.Client {
	return d.client
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Query — any row-returning statement, no verb guard.

func (d *DB) QueryRowRaw(ctx context.Context, query string, args ...any) sqldbs.Row {
	return &Row{row: d.pool.QueryRow(ctx, query, args...)}
}

func (d *DB) QueryRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

// Verb-guarded — still caller-written SQL, with a first-word check added.

func (d *DB) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return d.QueryRowRaw(ctx, query, args...), nil
}

func (d *DB) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	return d.QueryRowsRaw(ctx, query, args...)
}

func (d *DB) InsertRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return 0, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	return d.Exec(ctx, query, args...)
}

func (d *DB) UpdateRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return 0, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	return d.Exec(ctx, query, args...)
}

func (d *DB) DeleteRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return 0, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	return d.Exec(ctx, query, args...)
}

// Built — statement builders over a Table.

// Select

func (d *DB) SelectRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK, colNames ...string) (sqldbs.Row, error) {
	cols, err := table.Columns().Choose(colNames...)
	if err != nil {
		return nil, fmt.Errorf("SelectRow: %w", err)
	}
	if err := table.ValidatePK(pkValue); err != nil {
		return nil, fmt.Errorf("SelectRow: %w", err)
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", sqldbs.QuoteJoinIdentifiers(d.client, cols.Names()), d.client.QuoteIdentifier(table.Name()), wherePK(d.client, table, 1))
	return d.QueryRowRaw(ctx, query, pkValue...), nil
}

func (d *DB) SelectRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond, colNames ...string) (sqldbs.Rows, error) {
	cols, err := table.Columns().Choose(colNames...)
	if err != nil {
		return nil, fmt.Errorf("SelectRows: %w", err)
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(d.client, cols.Names()), d.client.QuoteIdentifier(table.Name()))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, 1)
		query += whereSQL
		args = whereArgs
	}
	return d.QueryRowsRaw(ctx, query, args...)
}

// Insert

func (d *DB) InsertRow(ctx context.Context, table *sqldbs.Table, columns []string, values []any) error {
	if len(columns) == 0 {
		return fmt.Errorf("InsertRow: no columns")
	}
	if len(columns) != len(values) {
		return fmt.Errorf("InsertRow: columns and values length mismatch")
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return fmt.Errorf("InsertRow: %w", err)
	}
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.client.QuoteIdentifier(table.Name()), sqldbs.QuoteJoinIdentifiers(d.client, columns), strings.Join(placeholders, ", "))
	_, err := d.Exec(ctx, query, values...)
	return err
}

// InsertRows bulk-loads through COPY ... FROM STDIN rather than building an
// INSERT. COPY streams the rows over the wire instead of parsing and planning a
// statement, so there is no statement-size or bind-parameter ceiling here: the
// row count a single call can carry is not practically bounded, and a caller
// need not chunk on this implementation's account.
//
// The trade is that COPY is not a statement, so nothing that attaches to one is
// available — no RETURNING, no ON CONFLICT. Adding either to InsertRows means
// giving up COPY for a multi-row INSERT and taking on its ceiling.
func (d *DB) InsertRows(ctx context.Context, table *sqldbs.Table, columns []string, rowValues [][]any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("InsertRows: no columns")
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return 0, fmt.Errorf("InsertRows: %w", err)
	}
	if len(rowValues) == 0 {
		return 0, nil
	}
	src := pgx.CopyFromRows(rowValues)
	// CopyFrom takes identifier parts and quotes them itself, so the name is
	// split here rather than passed through QuoteIdentifier — whose output would
	// then be quoted a second time.
	count, err := d.pool.CopyFrom(ctx, pgx.Identifier(strings.Split(table.Name(), ".")), columns, src)
	return count, err
}

func (d *DB) InsertRowAutoIncrementingPK(ctx context.Context, table *sqldbs.Table, columns []string, values []any) (int64, error) {
	query, err := buildInsertAutoIncrementingPK(d.client, table, columns)
	if err != nil {
		return 0, err
	}
	var pk int64
	if err := d.QueryRowRaw(ctx, query, values...).Scan(&pk); err != nil {
		return 0, err
	}
	return pk, nil
}

// Update

func (d *DB) UpdateRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK, columns []string, values []any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("UpdateRow: no columns")
	}
	if len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRow: columns and values length mismatch")
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return 0, fmt.Errorf("UpdateRow: %w", err)
	}
	if err := table.ValidatePK(pkValue); err != nil {
		return 0, fmt.Errorf("UpdateRow: %w", err)
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", d.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", d.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "), wherePK(d.client, table, len(columns)+1))
	// Built fresh rather than appended to: values belongs to the caller, and
	// appending would write the key into their backing array when it has room.
	args := make([]any, 0, len(values)+len(pkValue))
	args = append(args, values...)
	args = append(args, pkValue...)
	return d.Exec(ctx, query, args...)
}

func (d *DB) UpdateRows(ctx context.Context, table *sqldbs.Table, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return 0, fmt.Errorf("UpdateRows: %w", err)
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", d.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s", d.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		startNth := len(columns) + 1
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, startNth)
		query += whereSQL
		args = append(args, whereArgs...)
	}
	return d.Exec(ctx, query, args...)
}

// Delete

func (d *DB) DeleteRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK) (int64, error) {
	if err := table.ValidatePK(pkValue); err != nil {
		return 0, fmt.Errorf("DeleteRow: %w", err)
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", d.client.QuoteIdentifier(table.Name()), wherePK(d.client, table, 1))
	return d.Exec(ctx, query, pkValue...)
}

func (d *DB) DeleteRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", d.client.QuoteIdentifier(table.Name()))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, 1)
		query += whereSQL
		args = whereArgs
	}
	return d.Exec(ctx, query, args...)
}

// DB-specific

// Prepare validates the statement now, then hands back one that runs through the
// pool. The connection borrowed for validation is returned immediately — the
// statement keeps none. Preparing here is only to make malformed SQL an error
// from Prepare rather than a surprise at first use; the preparation that matters
// happens per connection, cached by the driver.
func (d *DB) Prepare(ctx context.Context, query string) (sqldbs.PreparedStmt, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	_, err = conn.Conn().Prepare(ctx, query, query) // name it by its own SQL: the driver caches under that key
	conn.Release()
	if err != nil {
		return nil, err
	}
	return &PreparedStmt{pool: d.pool, sql: query}, nil
}

func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Transaction

// BeginTx starts a transaction on a pool-owned connection. pool.Begin (not
// Acquire + conn.Begin) because the returned *pgxpool.Tx releases the
// connection back to the pool on Commit/Rollback — a conn acquired here has no
// owner afterwards and would leak a pool slot per transaction.
func (d *DB) BeginTx(ctx context.Context) (sqldbs.Tx, error) {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction failed: %w", err)
	}
	return &Tx{tx: tx, db: d}, nil
}

// Schema Inspection

func (d *DB) PKColumnOf(ctx context.Context, table string) (string, bool, error) {
	var colName string
	var hasDefault bool
	err := d.QueryRowRaw(ctx,
		`SELECT a.attname, COALESCE(pg_get_serial_sequence($1, a.attname), '') != ''
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
		LIMIT 1`, table,
	).Scan(&colName, &hasDefault)
	if err != nil {
		return "", false, fmt.Errorf("PKColumnOf %q: %w", table, err)
	}
	return colName, hasDefault, nil
}

// Raw SQL Store

func (d *DB) SetMainRawSQLStore(name string) {
	d.mainStore = d.client.stores[name]
}

func (d *DB) MainRawSQLStore() *sqldbs.RawSQLStore {
	if d.mainStore == nil {
		panic("MainRawSQLStore not set — call SetMainRawSQLStore at boot")
	}
	return d.mainStore
}
