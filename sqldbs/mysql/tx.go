package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.Tx implements sqldbs.Tx interface
var _ sqldbs.Tx = (*Tx)(nil)

type Tx struct {
	tx *sql.Tx
	db *DB
}

// Core

func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (t *Tx) Client() sqldbs.Client {
	return t.db.client
}

// Query — any row-returning statement, no verb guard.

func (t *Tx) QueryRowRaw(ctx context.Context, query string, args ...any) sqldbs.Row {
	return &Row{row: t.tx.QueryRowContext(ctx, query, args...)}
}

func (t *Tx) QueryRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

// Select

func (t *Tx) SelectRow(ctx context.Context, table string, pkColumn string, id any, columns []string) (sqldbs.Row, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRow: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = ? LIMIT 1", sqldbs.QuoteJoinIdentifiers(t.db.client, columns), t.db.client.QuoteIdentifier(table), t.db.client.QuoteIdentifier(pkColumn))
	return &Row{row: t.tx.QueryRowContext(ctx, query, id)}, nil
}

func (t *Tx) SelectRows(ctx context.Context, table string, columns []string, where sqldbs.Cond) (sqldbs.Rows, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRows: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(t.db.client, columns), t.db.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
	}
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (t *Tx) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return &Row{row: t.tx.QueryRowContext(ctx, query, args...)}, nil
}

func (t *Tx) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	rows, err := t.tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

// Insert

func (t *Tx) InsertRow(ctx context.Context, table string, columns []string, values []any) (sqldbs.Result, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("InsertRow: no columns")
	}
	placeholders := strings.Repeat("?, ", len(columns)-1) + "?"
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", t.db.client.QuoteIdentifier(table), sqldbs.QuoteJoinIdentifiers(t.db.client, columns), placeholders)
	result, err := t.tx.ExecContext(ctx, query, values...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (t *Tx) InsertRows(ctx context.Context, table string, columns []string, rowValues [][]any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("InsertRows: no columns")
	}
	if len(rowValues) == 0 {
		return 0, nil
	}
	colList := sqldbs.QuoteJoinIdentifiers(t.db.client, columns)
	placeholders := "(" + strings.Repeat("?, ", len(columns)-1) + "?)"
	allPlaceholders := strings.Repeat(placeholders+", ", len(rowValues)-1) + placeholders
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s", t.db.client.QuoteIdentifier(table), colList, allPlaceholders)
	var args []any
	for _, row := range rowValues {
		args = append(args, row...)
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (t *Tx) InsertRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return nil, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

// Update

func (t *Tx) UpdateRow(ctx context.Context, table string, pkColumn string, id any, columns []string, values []any) (sqldbs.Result, error) {
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = t.db.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = ?", t.db.client.QuoteIdentifier(table), strings.Join(setClauses, ", "), t.db.client.QuoteIdentifier(pkColumn))
	args := append(values, id)
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (t *Tx) UpdateRows(ctx context.Context, table string, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = t.db.client.QuoteIdentifier(col) + " = ?"
	}
	query := fmt.Sprintf("UPDATE %s SET %s", t.db.client.QuoteIdentifier(table), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = append(args, whereArgs...)
		}
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (t *Tx) UpdateRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return nil, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

// Delete

func (t *Tx) DeleteRow(ctx context.Context, table string, pkColumn string, id any) (sqldbs.Result, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = ?", t.db.client.QuoteIdentifier(table), t.db.client.QuoteIdentifier(pkColumn))
	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

func (t *Tx) DeleteRows(ctx context.Context, table string, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", t.db.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereRaw, whereArgs := where.BindRepr()
		if whereRaw != "" {
			query += " WHERE " + whereRaw
			args = whereArgs
		}
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (t *Tx) DeleteRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return nil, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	result, err := t.tx.ExecContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{result: result}, nil
}

// Tx-specific

func (t *Tx) DB() sqldbs.DB {
	return t.db
}

// Transaction Control

func (t *Tx) Commit(_ context.Context) error {
	return t.tx.Commit()
}

func (t *Tx) Rollback(_ context.Context) error {
	return t.tx.Rollback()
}
