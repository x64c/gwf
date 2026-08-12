package sqldbs

import (
	"fmt"
	"slices"
)

// Columns is an ordered set of columns, keyed by name.
//
// It is immutable: neither the map nor the order is handed out, so a Columns
// value can be passed around and read by any number of goroutines. Choose builds
// a new one rather than changing this one — which is what lets a whole catalog be
// rebuilt and swapped in atomically while readers carry on with the version they
// already hold.
//
// Every set is keyed, however it was made. A table may be arbitrarily wide, and
// lookups happen against whatever set is in hand, so leaving some of them to a
// scan would put the cost exactly where the width is.
//
// The zero value is an empty set: Len is 0, Find answers false, and Choose
// refuses any name.
type Columns struct {
	colsMap map[string]Column // uniqueness enforced by name
	order   []string          // the names in sequence
}

// newColumns takes ownership of items, which the caller must not keep.
func newColumns(items []Column) Columns {
	colsMap := make(map[string]Column, len(items))
	order := make([]string, len(items))
	for i, col := range items {
		colsMap[col.name] = col
		order[i] = col.name
	}
	return Columns{colsMap: colsMap, order: order}
}

// Len is how many columns are in the set.
func (c Columns) Len() int { return len(c.order) }

// Names are the column names in order — what a statement builder needs.
// The slice is freshly made, so the caller may keep or alter it.
func (c Columns) Names() []string { return slices.Clone(c.order) }

// Find looks a column up by name.
func (c Columns) Find(name string) (Column, bool) {
	col, ok := c.colsMap[name]
	return col, ok
}

// at is the ith column in order. The caller owns the bounds check — an index
// from anywhere but a loop over Len is a bug here, not a condition to soften.
func (c Columns) at(i int) Column { return c.colsMap[c.order[i]] }

// Choose narrows to the named columns, in the order given. That order is the
// caller's to decide and is preserved, since a caller scanning by position needs
// the statement to line up with its destinations.
//
// Choosing none returns the whole set. A name that is not in the set, or is
// asked for twice, is refused rather than reaching SQL.
func (c Columns) Choose(names ...string) (Columns, error) {
	if len(names) == 0 {
		return c, nil
	}
	chosen := make([]Column, 0, len(names))
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			return Columns{}, fmt.Errorf("column %q chosen more than once", name)
		}
		col, ok := c.Find(name)
		if !ok {
			return Columns{}, fmt.Errorf("no column %q", name)
		}
		seen[name] = true
		chosen = append(chosen, col)
	}
	return newColumns(chosen), nil
}
