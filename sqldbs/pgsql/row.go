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

func (r *Row) Scan(dest ...any) error {
	// first, scan to `int16`s instead of `bool`s
	raw := make([]any, len(dest))
	for i, d := range dest {
		switch d.(type) {
		case *bool:
			raw[i] = new(int16) // PostgreSQL's smallest integer type is smallint (2 bytes)
		default:
			raw[i] = d
		}
	}
	err := r.row.Scan(raw...)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return sqldbs.ErrNoRows
		}
		return err
	}
	// fill dest with `bool` as `bool`
	for i, d := range dest {
		switch v := d.(type) {
		case *bool:
			*v = *(raw[i].(*int16)) != 0
		}
	}
	return nil
}
