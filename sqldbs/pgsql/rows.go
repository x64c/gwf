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

// Scan passes dests straight to pgx: type mapping is pgx-native
// (Go bool ↔ boolean, etc.) — no cross-dialect value shims.
func (r *Rows) Scan(dest ...any) error {
	return r.current.Scan(dest...)
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
