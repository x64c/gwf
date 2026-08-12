package pgsql

import (
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// buildInsertAutoIncrementingPK validates an InsertRowAutoIncrementingPK call and
// builds the statement that asks the generated key back as it inserts.
func buildInsertAutoIncrementingPK(c *Client, table *sqldbs.Table, columns []string) (string, error) {
	if !table.PKAutoIncrement() {
		return "", fmt.Errorf("InsertRowAutoIncrementingPK: table %q declares no auto-increment primary key", table.Name())
	}
	pkCol, err := table.SinglePKColumn()
	if err != nil {
		return "", fmt.Errorf("InsertRowAutoIncrementingPK: %w", err)
	}
	if len(columns) == 0 {
		return "", fmt.Errorf("InsertRowAutoIncrementingPK: no columns")
	}
	for _, col := range columns {
		if col == pkCol.Name() {
			return "", fmt.Errorf("InsertRowAutoIncrementingPK: %q is the auto-increment primary key and must not be inserted", col)
		}
	}
	if _, err := table.Columns().Choose(columns...); err != nil {
		return "", fmt.Errorf("InsertRowAutoIncrementingPK: %w", err)
	}
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s) RETURNING %s",
		c.QuoteIdentifier(table.Name()),
		sqldbs.QuoteJoinIdentifiers(c, columns),
		strings.Join(placeholders, ", "),
		c.QuoteIdentifier(pkCol.Name()),
	), nil
}
