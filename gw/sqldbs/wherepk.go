package sqldbs

import "strings"

// WherePK renders the primary-key equality chain — one comparison per key
// column, in key order, so a PK's values bind to it positionally. A
// single-column key is the one-comparison case of the same chain, not a
// special case. Quoting and placeholders come from the client's dialect;
// startNth is the position of the chain's first bind argument within the
// whole statement (a dialect with anonymous placeholders renders it
// invisibly).
func WherePK(c Client, table *Table, startNth int) string {
	names := table.PKColumns().Names()
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = c.QuoteIdentifier(name) + " = " + c.NthPlaceholder(startNth+i)
	}
	return strings.Join(parts, " AND ")
}
