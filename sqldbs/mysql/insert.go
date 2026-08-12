package mysql

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/x64c/gwf/gw/sqldbs"
)

// buildInsertAutoIncrementingPK validates an InsertRowAutoIncrementingPK call and
// builds the statement. The key column takes no part in the SQL — the server
// reports the generated key on the connection — but the call is still checked
// against the table, so the same mistake fails the same way on every backend.
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
	placeholders := strings.Repeat("?, ", len(columns)-1) + "?"
	return fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		c.QuoteIdentifier(table.Name()),
		sqldbs.QuoteJoinIdentifiers(c, columns),
		placeholders,
	), nil
}

// autoIncrementPK reads the generated key off a finished insert. A generated
// key is never 0, so 0 means the table's primary key does not auto-increment —
// which the caller asserted it does.
func autoIncrementPK(result sql.Result, table string) (int64, error) {
	pk, err := result.LastInsertId()
	if err != nil {
		return 0, err
	}
	if pk == 0 {
		return 0, fmt.Errorf("InsertRowAutoIncrementingPK: %q reported no generated key; its primary key does not auto-increment", table)
	}
	return pk, nil
}
