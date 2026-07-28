package pgsql

import (
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.Row implements sqldbs.Row interface
var _ sqldbs.Row = (*Row)(nil)

type Row struct {
	row pgx.Row
}

// Scan passes dests straight to pgx: type mapping is pgx-native
// (Go bool ↔ boolean, etc.) — no cross-dialect value shims.
func (r *Row) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if errors.Is(err, pgx.ErrNoRows) {
		return sqldbs.ErrNoRows
	}
	return err
}
