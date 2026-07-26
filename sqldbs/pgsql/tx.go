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

func (t *Tx) Exec(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (t *Tx) Client() sqldbs.Client {
	return t.db.client
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

// Select

func (t *Tx) SelectRow(ctx context.Context, table string, pkColumn string, id any, columns []string) (sqldbs.Row, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRow: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1 LIMIT 1", sqldbs.QuoteJoinIdentifiers(t.db.client, columns), t.db.client.QuoteIdentifier(table), t.db.client.QuoteIdentifier(pkColumn))
	return &Row{row: t.tx.QueryRow(ctx, query, id)}, nil
}

func (t *Tx) SelectRows(ctx context.Context, table string, columns []string, where sqldbs.Cond) (sqldbs.Rows, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRows: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(t.db.client, columns), t.db.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, 1)
		query += whereSQL
		args = whereArgs
	}
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

func (t *Tx) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return &Row{row: t.tx.QueryRow(ctx, query, args...)}, nil
}

func (t *Tx) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

// Insert

func (t *Tx) InsertRow(ctx context.Context, table string, columns []string, values []any) (sqldbs.Result, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("InsertRow: no columns")
	}
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING id", t.db.client.QuoteIdentifier(table), sqldbs.QuoteJoinIdentifiers(t.db.client, columns), strings.Join(placeholders, ", "))
	var id int64
	err := t.tx.QueryRow(ctx, query, values...).Scan(&id)
	if err != nil {
		return nil, err
	}
	return &Result{lastInsertID: id}, nil
}

func (t *Tx) InsertRows(ctx context.Context, table string, columns []string, rowValues [][]any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("InsertRows: no columns")
	}
	if len(rowValues) == 0 {
		return 0, nil
	}
	src := pgx.CopyFromRows(rowValues)
	count, err := t.tx.CopyFrom(ctx, pgx.Identifier{table}, columns, src)
	return count, err
}

func (t *Tx) InsertRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return nil, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	if !strings.Contains(strings.ToUpper(query), "RETURNING") {
		query += " RETURNING id"
		var id int64
		err := t.tx.QueryRow(ctx, query, args...).Scan(&id)
		if err != nil {
			return nil, err
		}
		return &Result{lastInsertID: id}, nil
	}
	tag, err := t.tx.Exec(ctx, query, args...)
	return &Result{tag: tag}, err
}

// Update

func (t *Tx) UpdateRow(ctx context.Context, table string, pkColumn string, id any, columns []string, values []any) (sqldbs.Result, error) {
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", t.db.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", t.db.client.QuoteIdentifier(table), strings.Join(setClauses, ", "), t.db.client.QuoteIdentifier(pkColumn), len(columns)+1)
	args := append(values, id)
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (t *Tx) UpdateRows(ctx context.Context, table string, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", t.db.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s", t.db.client.QuoteIdentifier(table), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		startNth := len(columns) + 1
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, startNth)
		query += whereSQL
		args = append(args, whereArgs...)
	}
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (t *Tx) UpdateRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return nil, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

// Delete

func (t *Tx) DeleteRow(ctx context.Context, table string, pkColumn string, id any) (sqldbs.Result, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", t.db.client.QuoteIdentifier(table), t.db.client.QuoteIdentifier(pkColumn))
	tag, err := t.tx.Exec(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (t *Tx) DeleteRows(ctx context.Context, table string, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", t.db.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(t.db.client, 1)
		query += whereSQL
		args = whereArgs
	}
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (t *Tx) DeleteRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return nil, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	tag, err := t.tx.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
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
