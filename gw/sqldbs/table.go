package sqldbs

import "fmt"

// Table is a validated SQL table name.
// It cannot be created directly — only via NewTable().
type Table struct {
	name string // unexported → cannot bypass validation
}

// Name returns the table name string.
func (t Table) Name() string { return t.name }

func NewTable(name string) (Table, error) {
	if !IdentifierRegexp.MatchString(name) {
		return Table{}, fmt.Errorf("invalid SQL table name: %q", name)
	}
	return Table{name: name}, nil
}

func NewTableOrPanic(name string) Table {
	if !IdentifierRegexp.MatchString(name) {
		panic(fmt.Errorf("invalid SQL table name: %q", name))
	}
	return Table{name: name}
}
