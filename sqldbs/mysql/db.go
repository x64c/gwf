package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.DB implements sqldbs.DB interface
var _ sqldbs.DB = (*DB)(nil)

type DB struct {
	conn      *sql.DB
	client    *Client
	mainStore *sqldbs.RawSQLStore
}

// Core

func (d *DB) Exec(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (d *DB) Client() sqldbs.Client {
	return d.client
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

// Select

func (d *DB) SelectRow(ctx context.Context, table string, pkColumn string, id any, columns []string) (sqldbs.Row, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRow: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? LIMIT 1", sqldbs.QuoteJoinIdentifiers(d.client, columns), d.client.QuoteIdentifier(table), d.client.QuoteIdentifier(pkColumn))
	return &Row{row: d.conn.QueryRowContext(ctx, query, id)}, nil
}

func (d *DB) SelectRows(ctx context.Context, table string, columns []string, where sqldbs.Cond) (sqldbs.Rows, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRows: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(d.client, columns), d.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
	}
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (d *DB) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return &Row{row: d.conn.QueryRowContext(ctx, query, args...)}, nil
}

func (d *DB) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	rows, err := d.conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

// Insert

func (d *DB) InsertRow(ctx context.Context, table string, columns []string, values []any) (sqldbs.Result, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("InsertRow: no columns")
	}
	placeholders := strings.Repeat("?, ", len(columns)-1) + "?"
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.client.QuoteIdentifier(table), sqldbs.QuoteJoinIdentifiers(d.client, columns), placeholders)
	result, err := d.conn.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (d *DB) InsertRows(ctx context.Context, table string, columns []string, rowValues [][]any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("InsertRows: no columns")
	}
	if len(rowValues) == 0 {
		return 0, nil
	}
	colList := sqldbs.QuoteJoinIdentifiers(d.client, columns)
	placeholders := "(" + strings.Repeat("?, ", len(columns)-1) + "?)"
	allPlaceholders := strings.Repeat(placeholders+", ", len(rowValues)-1) + placeholders
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", d.client.QuoteIdentifier(table), colList, allPlaceholders)
	var args []any
	for _, row := range rowValues {
		args = append(args, row...)
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) InsertRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return nil, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

// Update

func (d *DB) UpdateRow(ctx context.Context, table string, pkColumn string, id any, columns []string, values []any) (sqldbs.Result, error) {
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = d.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", d.client.QuoteIdentifier(table), strings.Join(setClauses, ", "), d.client.QuoteIdentifier(pkColumn))
	args := append(values, id)
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (d *DB) UpdateRows(ctx context.Context, table string, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = d.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s", d.client.QuoteIdentifier(table), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = append(args, whereArgs...)
		}
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) UpdateRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return nil, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

// Delete

func (d *DB) DeleteRow(ctx context.Context, table string, pkColumn string, id any) (sqldbs.Result, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", d.client.QuoteIdentifier(table), d.client.QuoteIdentifier(pkColumn))
	result, err := d.conn.ExecContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (d *DB) DeleteRows(ctx context.Context, table string, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", d.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (d *DB) DeleteRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return nil, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	result, err := d.conn.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
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
	err := d.conn.QueryRowContext(ctx,
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
