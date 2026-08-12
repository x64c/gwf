package pgsql

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.Tx implements sqldbs.Tx interface
var _ sqldbs.Tx = (*Tx)(nil)

type Tx struct {
	tx pgx.Tx
	db *DB // parent db which creates this tx
}

// Core

func (t *Tx) Client() sqldbs.Client {
	return t.db.client
}

func (t *Tx) Exec(ctx context.Context, query string, args ...any) (int64, error) {
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Query — any row-returning statement, no verb guard.

func (t *Tx) QueryRowRaw(ctx context.Context, query string, args ...any) sqldbs.Row {
	return &Row{row: t.tx.QueryRow(ctx, query, args...)}
}

func (t *Tx) QueryRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

// Verb-guarded — still caller-written SQL, with a first-word check added.

func (t *Tx) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return t.QueryRowRaw(ctx, query, args...), nil
}

func (t *Tx) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	return t.QueryRowsRaw(ctx, query, args...)
}

func (t *Tx) InsertRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return 0, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	return t.Exec(ctx, query, args...)
}

func (t *Tx) UpdateRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return 0, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	return t.Exec(ctx, query, args...)
}

func (t *Tx) DeleteRowsRaw(ctx context.Context, query string, args ...any) (int64, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return 0, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	return t.Exec(ctx, query, args...)
}

// Built — statement builders over a Table.

// Select

func (t *Tx) SelectRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK, colNames ...string) (sqldbs.Row, error) {
	cols, err := table.Columns().Choose(colNames...)
	if err != nil {
		return nil, fmt.Errorf("SelectRow: %w", err)
	}
	if err := table.ValidatePK(pkValue); err != nil {
		return nil, fmt.Errorf("SelectRow: %w", err)
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s LIMIT 1", sqldbs.QuoteJoinIdentifiers(t.db.client, cols.Names()), t.db.client.QuoteIdentifier(table.Name()), wherePK(t.db.client, table, 1))
	return t.QueryRowRaw(ctx, query, pkValue...), nil
}

func (t *Tx) SelectRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond, colNames ...string) (sqldbs.Rows, error) {
	cols, err := table.Columns().Choose(colNames...)
	if err != nil {
		return nil, fmt.Errorf("SelectRows: %w", err)
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(t.db.client, cols.Names()), t.db.client.QuoteIdentifier(table.Name()))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, 1)
		query += whereSQL
		args = whereArgs
	}
	return t.QueryRowsRaw(ctx, query, args...)
}

// Insert

func (t *Tx) InsertRow(ctx context.Context, table *sqldbs.Table, columns []string, values []any) error {
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
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", t.db.client.QuoteIdentifier(table.Name()), sqldbs.QuoteJoinIdentifiers(t.db.client, columns), strings.Join(placeholders, ", "))
	_, err := t.Exec(ctx, query, values...)
	return err
}

func (t *Tx) InsertRows(ctx context.Context, table *sqldbs.Table, columns []string, rowValues [][]any) (int64, error) {
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
	count, err := t.tx.CopyFrom(ctx, pgx.Identifier(strings.Split(table.Name(), ".")), columns, src)
	return count, err
}

func (t *Tx) InsertRowAutoIncrementingPK(ctx context.Context, table *sqldbs.Table, columns []string, values []any) (int64, error) {
	query, err := buildInsertAutoIncrementingPK(t.db.client, table, columns)
	if err != nil {
		return 0, err
	}
	var pk int64
	if err := t.QueryRowRaw(ctx, query, values...).Scan(&pk); err != nil {
		return 0, err
	}
	return pk, nil
}

// Update

func (t *Tx) UpdateRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK, columns []string, values []any) (int64, error) {
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
		setClauses[i] = fmt.Sprintf("%s = $%d", t.db.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s", t.db.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "), wherePK(t.db.client, table, len(columns)+1))
	// Built fresh rather than appended to: values belongs to the caller, and
	// appending would write the key into their backing array when it has room.
	args := make([]any, 0, len(values)+len(pkValue))
	args = append(args, values...)
	args = append(args, pkValue...)
	return t.Exec(ctx, query, args...)
}

func (t *Tx) UpdateRows(ctx context.Context, table *sqldbs.Table, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return 0, fmt.Errorf("UpdateRows: %w", err)
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", t.db.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s", t.db.client.QuoteIdentifier(table.Name()), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		startNth := len(columns) + 1
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, startNth)
		query += whereSQL
		args = append(args, whereArgs...)
	}
	return t.Exec(ctx, query, args...)
}

// Delete

func (t *Tx) DeleteRow(ctx context.Context, table *sqldbs.Table, pkValue sqldbs.PK) (int64, error) {
	if err := table.ValidatePK(pkValue); err != nil {
		return 0, fmt.Errorf("DeleteRow: %w", err)
	}
	query := fmt.Sprintf("DELETE FROM %s WHERE %s", t.db.client.QuoteIdentifier(table.Name()), wherePK(t.db.client, table, 1))
	return t.Exec(ctx, query, pkValue...)
}

func (t *Tx) DeleteRows(ctx context.Context, table *sqldbs.Table, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", t.db.client.QuoteIdentifier(table.Name()))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, 1)
		query += whereSQL
		args = whereArgs
	}
	return t.Exec(ctx, query, args...)
}

// Tx-specific

func (t *Tx) DB() sqldbs.DB {
	return t.db
}

// Transaction Control

func (t *Tx) Commit(ctx context.Context) error {
	return t.tx.Commit(ctx)
}

func (t *Tx) Rollback(ctx context.Context) error {
	return t.tx.Rollback(ctx)
}
