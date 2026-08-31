package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// CachedSchema is the snapshot fetched at construction or by the last
// RefreshSchema — never nil, because PrepareDB fails rather than build a DB
// without one.
func (d *DB) CachedSchema() *sqldbs.Schema { return d.schema.Load() }

// RefreshSchema fetches fresh facts and swaps them in as the held snapshot.
func (d *DB) RefreshSchema(ctx context.Context) (*sqldbs.Schema, error) {
	schema, err := d.FetchSchema(ctx)
	if err != nil {
		return nil, err
	}
	d.schema.Store(schema)
	return schema, nil
}

// FetchSchema reads the connected database's ordinary tables from
// INFORMATION_SCHEMA — TABLE_TYPE 'BASE TABLE' only, so views are excluded.
//
// Defaults are as COLUMN_DEFAULT reports them; MySQL reports "no default" and
// an explicit DEFAULT NULL identically, so the two are not distinguished. A
// key column counts as database-assigned when its EXTRA carries
// auto_increment.
func (d *DB) FetchSchema(ctx context.Context) (*sqldbs.Schema, error) {
	rows, err := d.QueryRowsRaw(ctx,
		`SELECT c.TABLE_NAME, c.COLUMN_NAME, c.COLUMN_TYPE, c.IS_NULLABLE, c.EXTRA, c.COLUMN_DEFAULT
		FROM INFORMATION_SCHEMA.COLUMNS c
		JOIN INFORMATION_SCHEMA.TABLES t
			ON t.TABLE_SCHEMA = c.TABLE_SCHEMA AND t.TABLE_NAME = c.TABLE_NAME
		WHERE c.TABLE_SCHEMA = DATABASE() AND t.TABLE_TYPE = 'BASE TABLE'
		ORDER BY c.TABLE_NAME, c.ORDINAL_POSITION`)
	if err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}
	defer rows.Close()

	var tableOrder []string
	tableCols := make(map[string][]sqldbs.ColumnSchema)
	autoInc := make(map[string]map[string]bool)
	for rows.Next() {
		var table, column, colType, isNullable, extra string
		var colDefault sql.NullString
		if err := rows.Scan(&table, &column, &colType, &isNullable, &extra, &colDefault); err != nil {
			return nil, fmt.Errorf("FetchSchema: %w", err)
		}
		if _, seen := tableCols[table]; !seen {
			tableOrder = append(tableOrder, table)
			autoInc[table] = make(map[string]bool)
		}
		tableCols[table] = append(tableCols[table],
			sqldbs.NewColumnSchema(column, colType, isNullable == "YES", colDefault.Valid, colDefault.String))
		if strings.Contains(extra, "auto_increment") {
			autoInc[table][column] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}

	pkRows, err := d.QueryRowsRaw(ctx,
		`SELECT TABLE_NAME, COLUMN_NAME
		FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
		WHERE TABLE_SCHEMA = DATABASE() AND CONSTRAINT_NAME = 'PRIMARY'
		ORDER BY TABLE_NAME, ORDINAL_POSITION`)
	if err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}
	defer pkRows.Close()

	pks := make(map[string][]string)
	for pkRows.Next() {
		var table, column string
		if err := pkRows.Scan(&table, &column); err != nil {
			return nil, fmt.Errorf("FetchSchema: %w", err)
		}
		pks[table] = append(pks[table], column)
	}
	if err := pkRows.Err(); err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}

	tables := make([]sqldbs.TableSchema, 0, len(tableOrder))
	for _, name := range tableOrder {
		pk := pks[name]
		pkAutoIncrement := len(pk) == 1 && autoInc[name][pk[0]]
		tables = append(tables, sqldbs.NewTableSchema(name, pk, pkAutoIncrement, tableCols[name]))
	}
	return sqldbs.NewSchema(tables), nil
}
