package mysql

import (
	"database/sql"
	"errors"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.Row implements sqldbs.Row interface
var _ sqldbs.Row = (*Row)(nil)

type Row struct {
	row *sql.Row
}

func (r *Row) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if errors.Is(err, sql.ErrNoRows) {
		return sqldbs.ErrNoRows
	}
	return err
}
