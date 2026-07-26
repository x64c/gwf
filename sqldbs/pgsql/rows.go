package pgsql

import (
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.Rows implements sqldbs.Rows interface
var _ sqldbs.Rows = (*Rows)(nil)

type Rows struct {
	conn    *pgxpool.Conn
	current pgx.Rows
	batch   pgx.BatchResults
}

func (r *Rows) Next() bool {
	return r.current.Next()
}

func (r *Rows) Scan(dest ...any) error {
	raw := make([]any, len(dest))
	for i, d := range dest {
		switch d.(type) {
		case *bool:
			raw[i] = new(int16) // PostgreSQL's smallest integer type is smallint (2 bytes)
		default:
			raw[i] = d
		}
	}
	if err := r.current.Scan(raw...); err != nil {
		return err
	}
	for i, d := range dest {
		switch v := d.(type) {
		case *bool:
			*v = *(raw[i].(*int16)) != 0
		}
	}
	return nil
}

func (r *Rows) Close() error {
	if r.current != nil {
		r.current.Close()
	}
	if r.batch != nil {
		_ = r.batch.Close()
	}
	if r.conn != nil {
		r.conn.Release()
	}
	return nil
}

func (r *Rows) Err() error {
	return r.current.Err()
}

func (r *Rows) NextResultSet() bool {
	if r.batch == nil {
		return false
	}
	nextRows, err := r.batch.Query()
	if err != nil {
		// No more result sets or query failed
		return false
	}
	r.current = nextRows
	return true
}
