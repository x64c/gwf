package pgsql

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/sqldbs"
)

// CachedSchema is the snapshot fetched at construction or by the last
// RefreshSchema — never nil, because CreateDB fails rather than build a DB
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

// FetchSchema reads the ordinary and partitioned tables of the connection's
// current schema (the head of search_path) from the system catalogs — views
// and other namespaces are excluded.
//
// Types come through format_type, so they read as PostgreSQL spells them
// ("character varying(4)", "timestamp with time zone"). A serial key carries
// its nextval(...) default; an identity key reports database-assigned with no
// default, because identity is not a default in PostgreSQL's own accounting.
//
// Database-assigned means the engine binds the generator to the column —
// serial's owned sequence, or identity. A hand-wired DEFAULT nextval(...) on
// an unowned sequence is deliberately NOT that, however its values behave: it
// is reported as what it is declared to be, a default, with its expression
// visible.
func (d *DB) FetchSchema(ctx context.Context) (*sqldbs.Schema, error) {
	rows, err := d.QueryRowsRaw(ctx,
		`SELECT c.relname,
			a.attname,
			format_type(a.atttypid, a.atttypmod),
			NOT a.attnotnull,
			a.atthasdef,
			COALESCE(pg_get_expr(ad.adbin, ad.adrelid), ''),
			(a.attidentity <> '' OR pg_get_serial_sequence(quote_ident(n.nspname) || '.' || quote_ident(c.relname), a.attname) IS NOT NULL)
		FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_attribute a ON a.attrelid = c.oid
		LEFT JOIN pg_attrdef ad ON ad.adrelid = c.oid AND ad.adnum = a.attnum
		WHERE n.nspname = current_schema()
			AND c.relkind IN ('r', 'p')
			AND a.attnum > 0
			AND NOT a.attisdropped
		ORDER BY c.relname, a.attnum`)
	if err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}
	defer rows.Close()

	var tableOrder []string
	tableCols := make(map[string][]sqldbs.ColumnSchema)
	autoInc := make(map[string]map[string]bool)
	for rows.Next() {
		var table, column, colType, colDefault string
		var nullable, hasDefault, generated bool
		if err := rows.Scan(&table, &column, &colType, &nullable, &hasDefault, &colDefault, &generated); err != nil {
			return nil, fmt.Errorf("FetchSchema: %w", err)
		}
		if _, seen := tableCols[table]; !seen {
			tableOrder = append(tableOrder, table)
			autoInc[table] = make(map[string]bool)
		}
		tableCols[table] = append(tableCols[table],
			sqldbs.NewColumnSchema(column, colType, nullable, hasDefault, colDefault))
		if generated {
			autoInc[table][column] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("FetchSchema: %w", err)
	}

	pkRows, err := d.QueryRowsRaw(ctx,
		`SELECT c.relname, a.attname
		FROM pg_index i
		JOIN pg_class c ON c.oid = i.indrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		CROSS JOIN LATERAL unnest(i.indkey) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE n.nspname = current_schema() AND i.indisprimary
		ORDER BY c.relname, k.ord`)
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
