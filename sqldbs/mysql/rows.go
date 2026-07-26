package mysql

import (
	"database/sql"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.Rows implements sqldbs.Rows interface
var _ sqldbs.Rows = (*Rows)(nil)

type Rows struct {
	rows *sql.Rows
}

func (r *Rows) Next() bool {
	return r.rows.Next()
}

func (r *Rows) Scan(dest ...any) error {
	return r.rows.Scan(dest...)
}

func (r *Rows) Close() error {
	return r.rows.Close()
}

func (r *Rows) Err() error {
	return r.rows.Err()
}

func (r *Rows) NextResultSet() bool {
	// MySQL via database/sql doesn't support multiple result sets here
	return false
}
