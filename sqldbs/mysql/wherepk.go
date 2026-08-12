package mysql

import (
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// wherePK renders the primary key equality chain — `k1` = ? AND `k2` = ? —
// one comparison per key column, in key order, so a PK's values bind to it
// positionally. A single-column key is the one-comparison case of the same
// chain, not a special case.
func wherePK(c *Client, table *sqldbs.Table) string {
	names := table.PKColumns().Names()
	parts := make([]string, len(names))
	for i, name := range names {
		parts[i] = c.QuoteIdentifier(name) + " = ?"
	}
	return strings.Join(parts, " AND ")
}
