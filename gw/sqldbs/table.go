package sqldbs

import "fmt"

// Table describes a database table: its name, its primary key — one column or
// several — whether the database assigns that key, and the other columns it
// has. One per table, not one per row — build it as a package-level var.
//
// Its fields are unexported and it has no setters, so a Table cannot change once
// built. That is the point of the type: one *Table is read by every goroutine
// serving a request, so a value that could be written after boot would be an
// unsynchronized write against the whole request path.
type Table struct {
	name            string  // table name, validated once here
	pkColumns       Columns // the primary key, in key order — len 1 unless composite
	pkAutoIncrement bool    // whether the database assigns the key (single-column keys only)

	// One truth in two shapes. Both get asked for, the constructor builds both,
	// and neither can drift because a Table never changes after it.
	columns      Columns // every column, the key first
	nonPKColumns Columns // the same without the key
}

// NewTable describes a table, rejecting a table name or column that is not a
// valid SQL identifier. (e.g. an empty string)
//
// pkColumns is the primary key, in key order — one column for most tables,
// several for a composite key. One form covers every key shape, mirroring PK
// on the value side: the nth PK value belongs to the nth key column declared
// here. A composite key never auto-increments — a generated key is a single
// column's property — so pkAutoIncrement requires exactly one key column.
//
// A valid identifier is not a correct one. Whether the table exists, whether
// these columns really are its primary key, and whether that key really is
// database-assigned are questions only the database can answer.
//
// nonPKColumns: every column except the key, in the order a whole row is read.
// They reach callers as Columns, to narrow with Choose.
func NewTable(name string, pkColumns []string, pkAutoIncrement bool, nonPKColumns []string) (*Table, error) {
	if err := ValidateIdentifier(name); err != nil {
		return nil, fmt.Errorf("NewTable: invalid table name %q: %w", name, err)
	}
	if len(pkColumns) == 0 {
		return nil, fmt.Errorf("NewTable: table %q declares no primary key column", name)
	}
	if pkAutoIncrement && len(pkColumns) != 1 {
		return nil, fmt.Errorf("NewTable: table %q declares a %d-column primary key as auto-increment; only a single column can be", name, len(pkColumns))
	}
	seen := make(map[string]bool, len(pkColumns)+len(nonPKColumns))
	pkCols := make([]Column, 0, len(pkColumns))
	for _, colName := range pkColumns {
		col, err := NewColumn(colName)
		if err != nil {
			return nil, fmt.Errorf("NewTable: table %q has an invalid primary key column: %w", name, err)
		}
		if seen[colName] {
			return nil, fmt.Errorf("NewTable: table %q names column %q twice (the primary key counts)", name, colName)
		}
		seen[colName] = true
		pkCols = append(pkCols, col)
	}
	cols := make([]Column, 0, len(nonPKColumns))
	for _, colName := range nonPKColumns {
		col, err := NewColumn(colName)
		if err != nil {
			return nil, fmt.Errorf("NewTable: table %q has an invalid column: %w", name, err)
		}
		if seen[colName] {
			return nil, fmt.Errorf("NewTable: table %q names column %q twice (the primary key counts)", name, colName)
		}
		seen[colName] = true
		cols = append(cols, col)
	}
	all := make([]Column, 0, len(pkCols)+len(cols))
	all = append(all, pkCols...) // the key leads, so a whole-row read starts with it
	all = append(all, cols...)

	return &Table{
		name:            name,
		pkColumns:       newColumns(pkCols),
		pkAutoIncrement: pkAutoIncrement,
		columns:         newColumns(all),
		nonPKColumns:    newColumns(cols),
	}, nil
}

// NewTableOrPanic is NewTable for a package-level var, where an error has
// nowhere to go.
// WARNING: This function panics if any identifier is invalid. Package
// initialization runs before main, so the failure lands as a failed program
// load — nothing has bound a listener or opened a pool yet.
//
// This is the ONLY panicking constructor in the package, and deliberately so.
// Column has no such variant: a bad identifier reached inline in a handler
// should be an error, not a panicked request. A Table is different only because
// it is declared once as a package var, where nothing can receive an error.
func NewTableOrPanic(name string, pkColumns []string, pkAutoIncrement bool, nonPKColumns []string) *Table {
	tbl, err := NewTable(name, pkColumns, pkAutoIncrement, nonPKColumns)
	if err != nil {
		panic(err)
	}
	return tbl
}

// Name is the table name, already validated.
func (m *Table) Name() string { return m.name }

// PKColumns is the primary key, in key order — one column for most tables,
// several for a composite key. Already validated; callers use the columns
// directly rather than re-deriving them per query.
func (m *Table) PKColumns() Columns { return m.pkColumns }

// PKAutoIncrement reports whether the primary key is assigned by the database
// rather than supplied with the row. Only a single-column key can be — the
// constructor enforces it.
func (m *Table) PKAutoIncrement() bool { return m.pkAutoIncrement }

// Columns is every column the table has, the primary key first.
// Narrow it with Columns().Choose(…).
func (m *Table) Columns() Columns { return m.columns }

// NonPKColumns is the same without the key — what an insert whose key the
// database assigns, or an UPDATE's SET list, is made of.
func (m *Table) NonPKColumns() Columns { return m.nonPKColumns }

// ValidatePK reports whether pk can name a row of this table: exactly one
// value per key column, in key order. It checks shape, not existence —
// whether such a row is present only the database can answer.
func (m *Table) ValidatePK(pk PK) error {
	if len(pk) != m.pkColumns.Len() {
		return fmt.Errorf("table %q has a %d-column primary key; got %d values", m.name, m.pkColumns.Len(), len(pk))
	}
	return nil
}

// SinglePKColumn is the key column of a single-column key, and an error on a
// composite one. It exists for operations built on one key value — the model
// layer's single identity (GetID), or an auto-increment key, which is a single
// column's property — so their implementations state the requirement by
// calling this rather than assuming it. Callers wrap the error with their own
// context.
func (m *Table) SinglePKColumn() (Column, error) {
	if m.pkColumns.Len() != 1 {
		return Column{}, fmt.Errorf("table %q has a %d-column primary key; this operation requires a single-column key", m.name, m.pkColumns.Len())
	}
	return m.pkColumns.at(0), nil
}

// SyncFromDB read the primary key from the live schema and overwrote this Table
// with it. Withdrawn, kept here until its replacement exists — delete this block
// once the verify pass lands.
//
// Three things were wrong with it. It WROTE a value the whole request path
// reads, against a type whose immutability is the reason it has no setters. It
// had no callers anywhere in the tree. And it ran the wrong way round: the code
// conformed to whatever schema it found instead of declaring what it required,
// so a renamed column silently became a different application rather than a
// failure.
//
// It is also no longer implementable as written. A Table now derives Columns and
// NonPKColumns in its constructor, so writing the key alone would leave a Table
// whose key disagrees with its own column sets.
//
// The replacement COMPARES declaration against schema and reports every
// mismatch at boot, leaving the Table untouched.
//
//	func (m *Table) SyncFromDB(ctx context.Context, db DB) error {
//		col, incr, err := db.PKColumnOf(ctx, m.name)
//		if err != nil {
//			return err
//		}
//		pkCol, err := NewColumn(col)
//		if err != nil {
//			return err
//		}
//		m.pkColumn = pkCol
//		m.pkAutoIncrement = incr
//		return nil
//	}
