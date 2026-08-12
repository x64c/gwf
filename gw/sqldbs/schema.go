package sqldbs

import "slices"

// The schema types are FACTS: what a database reports about itself, recorded
// as it states them. Nothing here passes through this package's identifier
// rules — a Table is a claim and gets validated; a Schema is what claims are
// validated AGAINST, and judging it would be backwards.
//
// All three types are immutable snapshots. Accessors hand out copies, so a
// caller can keep or alter what it receives without touching the snapshot.

// ColumnSchema is one column as the database reports it: its name, the
// dialect's own spelling of its type, whether it may hold NULL, and its
// default.
type ColumnSchema struct {
	name        string
	dataType    string
	nullable    bool
	hasDefault  bool
	defaultExpr string
}

// NewColumnSchema records a reported column. defaultExpr is meaningful only
// with hasDefault true.
func NewColumnSchema(name string, dataType string, nullable bool, hasDefault bool, defaultExpr string) ColumnSchema {
	return ColumnSchema{name: name, dataType: dataType, nullable: nullable, hasDefault: hasDefault, defaultExpr: defaultExpr}
}

// Name is the column's name as reported.
func (c ColumnSchema) Name() string { return c.name }

// DataType is the column's type in the dialect's own spelling. Two databases
// report the same logical column differently; nothing normalizes this, so
// compare it only with something that knows the dialect.
func (c ColumnSchema) DataType() string { return c.dataType }

// Nullable reports whether the column may hold NULL.
func (c ColumnSchema) Nullable() bool { return c.nullable }

// HasDefault reports whether the database fills this column on an insert
// that leaves it out. This bool is the fact — it cannot be read off
// DefaultExpr, because an empty expression is itself a legitimate default.
// A column with neither a default nor NULL to fall back on must be supplied
// by every insert.
func (c ColumnSchema) HasDefault() bool { return c.hasDefault }

// DefaultExpr is the default in the dialect's own spelling, unnormalized —
// a literal, an expression, or a generator, however the database states it.
// Meaningful only when HasDefault reports true.
func (c ColumnSchema) DefaultExpr() string { return c.defaultExpr }

// TableSchema is one table as the database reports it: every column in
// ordinal order, and the primary key — its columns in key order, and whether
// a single-column key is database-assigned. A table without a primary key
// reports no key columns.
type TableSchema struct {
	name            string
	pkColumns       []string
	pkAutoIncrement bool
	columns         []ColumnSchema
	byName          map[string]int // index into columns
}

// NewTableSchema records a reported table. columns arrive in the table's
// ordinal order and pkColumns in key order; both are kept as given.
func NewTableSchema(name string, pkColumns []string, pkAutoIncrement bool, columns []ColumnSchema) TableSchema {
	byName := make(map[string]int, len(columns))
	for i, col := range columns {
		byName[col.name] = i
	}
	return TableSchema{
		name:            name,
		pkColumns:       slices.Clone(pkColumns),
		pkAutoIncrement: pkAutoIncrement,
		columns:         slices.Clone(columns),
		byName:          byName,
	}
}

// Name is the table's name as reported.
func (t TableSchema) Name() string { return t.name }

// PKColumns is the primary key's column names in key order — empty when the
// table has no primary key.
func (t TableSchema) PKColumns() []string { return slices.Clone(t.pkColumns) }

// PKAutoIncrement reports whether the database assigns the key's value on an
// insert that leaves it out. Only a single-column key can be.
func (t TableSchema) PKAutoIncrement() bool { return t.pkAutoIncrement }

// Columns is every column in the table's ordinal order.
func (t TableSchema) Columns() []ColumnSchema { return slices.Clone(t.columns) }

// Column looks a column up by name.
func (t TableSchema) Column(name string) (ColumnSchema, bool) {
	i, ok := t.byName[name]
	if !ok {
		return ColumnSchema{}, false
	}
	return t.columns[i], true
}

// Len is how many columns the table has.
func (t TableSchema) Len() int { return len(t.columns) }

// Schema is one database's tables at one moment — the snapshot FetchSchema
// hands back. It states facts for anyone who asks: a boot-time verifier
// checking model declarations, a dev endpoint dumping it, a tool diffing it.
// It knows nothing about models, and holding one keeps nothing current: ask
// again for fresher facts.
type Schema struct {
	tables map[string]TableSchema
	names  []string // in the order the database reported them
}

// NewSchema records a reported set of tables, kept in the order given.
func NewSchema(tables []TableSchema) *Schema {
	byName := make(map[string]TableSchema, len(tables))
	names := make([]string, len(tables))
	for i, tbl := range tables {
		byName[tbl.name] = tbl
		names[i] = tbl.name
	}
	return &Schema{tables: byName, names: names}
}

// Table looks a table up by name.
func (s *Schema) Table(name string) (TableSchema, bool) {
	tbl, ok := s.tables[name]
	return tbl, ok
}

// Names is every table name, in the order the database reported them.
func (s *Schema) Names() []string { return slices.Clone(s.names) }

// Len is how many tables the snapshot holds.
func (s *Schema) Len() int { return len(s.names) }
