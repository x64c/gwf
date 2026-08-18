package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.DB implements sqldbs.DB interface
var _ sqldbs.DB = (*DB)(nil)

type DB struct {
	conn      *sql.DB
	client    *Client
	mainStore *sqldbs.RawSQLStore
	schema    atomic.Pointer[sqldbs.Schema] // held snapshot — see CachedSchema
}

// Core

func (d *DB) Client() sqldbs.Client {
	return d.client
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// Query — any row-returning statement, no verb guard.

func (d *DB) QueryRowRaw(ctx context.Context, query string, args ...any) sqldbs.Row {
	return &Row{row: d.conn.QueryRowContext(ctx, query, args...)}
}

func (d *DB) QueryRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

// Verb-guarded — still caller-written SQL, with a first-word check added.

func (d *DB) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	if err := sqldbs.CheckRawVerb("SelectRowRaw", "SELECT", query); err != nil {
		return nil, err
	}
	return d.QueryRowRaw(ctx, query, args...), nil
}

func (d *DB) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	if err := sqldbs.CheckRawVerb("SelectRowsRaw", "SELECT", query); err != nil {
		return nil, err
	}
	return d.QueryRowsRaw(ctx, query, args...)
}

func (d *DB) InsertRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	if err := sqldbs.CheckRawVerb("InsertRowsRaw", "INSERT", query); err != nil {
		return 0, err
	}
	return d.Exec(ctx, query, args...)
}

func (d *DB) UpdateRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	if err := sqldbs.CheckRawVerb("UpdateRowsRaw", "UPDATE", query); err != nil {
		return 0, err
	}
	return d.Exec(ctx, query, args...)
}

func (d *DB) DeleteRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	if err := sqldbs.CheckRawVerb("DeleteRowsRaw", "DELETE", query); err != nil {
		return 0, err
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
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", sqldbs.QuoteJoinIdentifiers(d.client, cols.Names()), d.client.QuoteIdentifier(table.Name()), sqldbs.WherePK(d.client, table, 1))
	return d.QueryRowRaw(ctx, query, pkValue...), nil
}

func (d *DB) SelectRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond, colNames ...string) (sqldbs.Rows, error) {
	cols, err := table.Columns().Choose(colNames...)
	if err != nil {
		return nil, fmt.Errorf("SelectRows: %w", err)
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(d.client, cols.Names()), d.client.QuoteIdentifier(table.Name()))
	// MySQL consumes the generic `?` placeholders exactly as BindRepr emits
	// them — nothing to translate, so the raw concat is the complete WHERE
	// build (sqldbs.WhereClause.Build exists for positional dialects). Same
	// in UpdateRows and DeleteRows.
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
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
	placeholders := strings.Repeat("?, ", len(columns)-1) + "?"
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.client.QuoteIdentifier(table.Name()), sqldbs.QuoteJoinIdentifiers(d.client, columns), placeholders)
	_, err := d.Exec(ctx, query, values...)
	return err
}

// InsertRows builds one multi-row INSERT. MySQL's bulk-load path, LOAD DATA
// INFILE, wants a file or a client-side stream and is commonly disabled on the
// server, so a statement is the workable choice here.
//
// Being a statement, it has a ceiling: every value is a bind parameter and the
// whole thing must fit the server's max_allowed_packet. A caller inserting a
// large set should chunk it — the interface says the bound belongs to the
// implementation, and this is that bound.
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
	colList := sqldbs.QuoteJoinIdentifiers(d.client, columns)
	placeholders := "(" + strings.Repeat("?, ", len(columns)-1) + "?)"
	allPlaceholders := strings.Repeat(placeholders+", ", len(rowValues)-1) + placeholders
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", d.client.QuoteIdentifier(table.Name()), colList, allPlaceholders)
	var args []any
	for _, row := range rowValues {
		args = append(args, row...)
	}
	return d.Exec(ctx, query, args...)
}

func (d *DB) InsertRowAutoIncrementingPK(ctx context.Context, table *sqldbs.Table, columns []string, values []any) (int64, error) {
	query, err := buildInsertAutoIncrementingPK(d.client, table, columns)
	if err != nil {
		return 0, err
	}
	// Stays on the connection rather than funneling through Exec: the generated
	// key comes off the driver result (LastInsertId), which Exec does not expose.
	result, err := d.conn.ExecContext(ctx, query, values...)
	if err != nil {
		return 0, err
	}
	return autoIncrementPK(result, table.Name())
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
		setClauses[i] = d.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", d.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "), sqldbs.WherePK(d.client, table, len(columns)+1))
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
		setClauses[i] = d.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s", d.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = append(args, whereArgs...)
		}
	}
	return d.Exec(ctx, query, args...)
}

// Delete

func (d *DB) DeleteRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK) (int64, error) {
	if err := table.ValidatePK(pkValue); err != nil {
		return 0, fmt.Errorf("DeleteRow: %w", err)
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", d.client.QuoteIdentifier(table.Name()), sqldbs.WherePK(d.client, table, 1))
	return d.Exec(ctx, query, pkValue...)
}

func (d *DB) DeleteRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", d.client.QuoteIdentifier(table.Name()))
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
	}
	return d.Exec(ctx, query, args...)
}

// DB-specific

func (d *DB) Prepare(ctx context.Context, query string) (sqldbs.PreparedStmt, error) {
	stmt, err := d.conn.PrepareContext(ctx, query)
	if err != nil {
		return nil, err
	}
	return &PreparedStmt{stmt: stmt}, nil
}

func (d *DB) Ping(ctx context.Context) error {
	return d.conn.PingContext(ctx)
}

// Transaction

func (d *DB) BeginTx(ctx context.Context) (sqldbs.Tx, error) {
	tx, err := d.conn.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &Tx{tx: tx, db: d}, nil
}

// Schema Inspection

func (d *DB) PKColumnOf(ctx context.Context, table string) (string, bool, error) {
	var colName, extra string
	err := d.QueryRowRaw(ctx,
		`SELECT c.COLUMN_NAME, c.EXTRA
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE k
		JOIN INFORMATION_SCHEMA.COLUMNS c
			ON c.TABLE_SCHEMA = k.TABLE_SCHEMA AND c.TABLE_NAME = k.TABLE_NAME AND c.COLUMN_NAME = k.COLUMN_NAME
		WHERE k.TABLE_SCHEMA = DATABASE() AND k.TABLE_NAME = ? AND k.CONSTRAINT_NAME = 'PRIMARY'
		LIMIT 1`, table,
	).Scan(&colName, &extra)
	if err != nil {
		return "", false, fmt.Errorf("PKColumnOf %q: %w", table, err)
	}
	return colName, strings.Contains(extra, "auto_increment"), nil
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
