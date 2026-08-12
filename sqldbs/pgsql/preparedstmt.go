package pgsql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.PreparedStmt implements sqldbs.PreparedStmt interface
var _ sqldbs.PreparedStmt = (*PreparedStmt)(nil)

// PreparedStmt holds the statement text and the pool, not a connection.
//
// This driver's default query mode already prepares and caches statements per
// connection, so pinning one to keep a statement "prepared" would buy nothing
// and cost a connection for the statement's whole life. Going through the pool
// instead prepares it once per connection that serves it, and is what makes the
// statement safe to share: each call takes a connection and gives it straight
// back, so no two callers meet on the wire.
type PreparedStmt struct {
	pool *pgxpool.Pool
	sql  string
}

func (p *PreparedStmt) Query(ctx context.Context, args ...any) (sqldbs.Rows, error) {
	rows, err := p.pool.Query(ctx, p.sql, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{current: rows}, nil
}

func (p *PreparedStmt) Exec(ctx context.Context, args ...any) (int64, error) {
	tag, err := p.pool.Exec(ctx, p.sql, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Close has nothing to release, since the statement holds no connection. Callers
// must go on calling it: another implementation may hold something.
func (p *PreparedStmt) Close() error { return nil }
