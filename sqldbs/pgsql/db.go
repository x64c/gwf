package pgsql

import (
	"context"
	"fmt"
	"strings"
	"time"

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
}

// Core

func (d *DB) Exec(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (d *DB) Client() sqldbs.Client {
	return d.client
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

// Select

func (d *DB) SelectRow(ctx context.Context, table string, pkColumn string, id any, columns []string) (sqldbs.Row, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRow: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s = $1 LIMIT 1", sqldbs.QuoteJoinIdentifiers(d.client, columns), d.client.QuoteIdentifier(table), d.client.QuoteIdentifier(pkColumn))
	return &Row{row: d.pool.QueryRow(ctx, query, id)}, nil
}

func (d *DB) SelectRows(ctx context.Context, table string, columns []string, where sqldbs.Cond) (sqldbs.Rows, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("SelectRows: no columns")
	}
	query := fmt.Sprintf("SELECT %s FROM %s", sqldbs.QuoteJoinIdentifiers(d.client, columns), d.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, 1)
		query += whereSQL
		args = whereArgs
	}
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

func (d *DB) SelectRowRaw(ctx context.Context, query string, args ...any) (sqldbs.Row, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowRaw: query must start with SELECT")
	}
	return &Row{row: d.pool.QueryRow(ctx, query, args...)}, nil
}

func (d *DB) SelectRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Rows, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "SELECT") {
		return nil, fmt.Errorf("SelectRowsRaw: query must start with SELECT")
	}
	rows, err := d.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{conn: nil, current: rows, batch: nil}, nil
}

// Insert

func (d *DB) InsertRow(ctx context.Context, table string, columns []string, values []any) (sqldbs.Result, error) {
	if len(columns) == 0 {
		return nil, fmt.Errorf("InsertRow: no columns")
	}
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", d.client.QuoteIdentifier(table), sqldbs.QuoteJoinIdentifiers(d.client, columns), strings.Join(placeholders, ", "))
	if !strings.Contains(strings.ToUpper(query), "RETURNING") {
		query += " RETURNING id"
		var id int64
		err := d.pool.QueryRow(ctx, query, values...).Scan(&id)
		if err != nil {
			return nil, err
		}
		return &Result{lastInsertID: id}, nil
	}
	tag, err := d.pool.Exec(ctx, query, values...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (d *DB) InsertRows(ctx context.Context, table string, columns []string, rowValues [][]any) (int64, error) {
	if len(columns) == 0 {
		return 0, fmt.Errorf("InsertRows: no columns")
	}
	if len(rowValues) == 0 {
		return 0, nil
	}
	src := pgx.CopyFromRows(rowValues)
	count, err := d.pool.CopyFrom(ctx, pgx.Identifier{table}, columns, src)
	return count, err
}

func (d *DB) InsertRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "INSERT") {
		return nil, fmt.Errorf("InsertRowsRaw: query must start with INSERT")
	}
	if !strings.Contains(strings.ToUpper(query), "RETURNING") {
		query += " RETURNING id"
		var id int64
		err := d.pool.QueryRow(ctx, query, args...).Scan(&id)
		if err != nil {
			return nil, err
		}
		return &Result{lastInsertID: id}, nil
	}
	tag, err := d.pool.Exec(ctx, query, args...)
	return &Result{tag: tag}, err
}

// Update

func (d *DB) UpdateRow(ctx context.Context, table string, pkColumn string, id any, columns []string, values []any) (sqldbs.Result, error) {
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", d.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s = $%d", d.client.QuoteIdentifier(table), strings.Join(setClauses, ", "), d.client.QuoteIdentifier(pkColumn), len(columns)+1)
	args := append(values, id)
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (d *DB) UpdateRows(ctx context.Context, table string, columns []string, values []any, where sqldbs.Cond) (int64, error) {
	if len(columns) == 0 || len(columns) != len(values) {
		return 0, fmt.Errorf("UpdateRows: columns and values length mismatch")
	}
	setClauses := make([]string, len(columns))
	for i, col := range columns {
		setClauses[i] = fmt.Sprintf("%s = $%d", d.client.QuoteIdentifier(col), i+1)
	}
	query := fmt.Sprintf("UPDATE %s SET %s", d.client.QuoteIdentifier(table), strings.Join(setClauses, ", "))
	args := make([]any, len(values))
	copy(args, values)
	if where != nil {
		startNth := len(columns) + 1
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, startNth)
		query += whereSQL
		args = append(args, whereArgs...)
	}
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *DB) UpdateRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "UPDATE") {
		return nil, fmt.Errorf("UpdateRowsRaw: query must start with UPDATE")
	}
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

// Delete

func (d *DB) DeleteRow(ctx context.Context, table string, pkColumn string, id any) (sqldbs.Result, error) {
	query := fmt.Sprintf("DELETE FROM %s WHERE %s = $1", d.client.QuoteIdentifier(table), d.client.QuoteIdentifier(pkColumn))
	tag, err := d.pool.Exec(ctx, query, id)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (d *DB) DeleteRows(ctx context.Context, table string, where sqldbs.Cond) (int64, error) {
	query := fmt.Sprintf("DELETE FROM %s", d.client.QuoteIdentifier(table))
	var args []any
	if where != nil {
		whereSQL, whereArgs := sqldbs.WhereClause{Cond: where}.Build(d.client, 1)
		query += whereSQL
		args = whereArgs
	}
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (d *DB) DeleteRowsRaw(ctx context.Context, query string, args ...any) (sqldbs.Result, error) {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), "DELETE") {
		return nil, fmt.Errorf("DeleteRowsRaw: query must start with DELETE")
	}
	tag, err := d.pool.Exec(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

// DB-specific

func (d *DB) Prepare(ctx context.Context, query string) (sqldbs.PreparedStmt, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	stmtName := fmt.Sprintf("stmt_%x", time.Now().UnixNano())
	_, err = conn.Conn().Prepare(ctx, stmtName, query)
	if err != nil {
		conn.Release()
		return nil, err
	}
	return &PreparedStmt{conn: conn, stmtName: stmtName}, nil
}

func (d *DB) Ping(ctx context.Context) error {
	return d.pool.Ping(ctx)
}

// Transaction

func (d *DB) BeginTx(ctx context.Context) (sqldbs.Tx, error) {
	conn, err := d.pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire connection failed: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		conn.Release()
		return nil, fmt.Errorf("begin transaction failed: %w", err)
	}
	return &Tx{tx: tx, db: d}, nil
}

// Schema Inspection

func (d *DB) PKColumnOf(ctx context.Context, table string) (string, bool, error) {
	var colName string
	var hasDefault bool
	err := d.pool.QueryRow(ctx,
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
