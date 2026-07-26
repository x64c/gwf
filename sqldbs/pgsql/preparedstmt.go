package pgsql

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure pgsql.PreparedStmt implements sqldbs.PreparedStmt interface
var _ sqldbs.PreparedStmt = (*PreparedStmt)(nil)

type PreparedStmt struct {
	conn     *pgxpool.Conn
	stmtName string
}

func (p *PreparedStmt) Query(ctx context.Context, args ...any) (sqldbs.Rows, error) {
	rows, err := p.conn.Query(ctx, p.stmtName, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{current: rows}, nil
}

func (p *PreparedStmt) Exec(ctx context.Context, args ...any) (sqldbs.Result, error) {
	tag, err := p.conn.Exec(ctx, p.stmtName, args...)
	if err != nil {
		return nil, err
	}
	return &Result{tag: tag}, nil
}

func (p *PreparedStmt) Close() error {
	p.conn.Release()
	return nil
}
