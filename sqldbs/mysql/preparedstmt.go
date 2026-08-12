package mysql

import (
	"context"
	"database/sql"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.PreparedStmt implements sqldbs.PreparedStmt interface
var _ sqldbs.PreparedStmt = (*PreparedStmt)(nil)

type PreparedStmt struct {
	stmt *sql.Stmt
}

func (p *PreparedStmt) Query(ctx context.Context, args ...any) (sqldbs.Rows, error) {
	rows, err := p.stmt.QueryContext(ctx, args...)
	if err != nil {
		return nil, err
	}
	return &Rows{rows: rows}, nil
}

func (p *PreparedStmt) Exec(ctx context.Context, args ...any) (int64, error) {
	result, err := p.stmt.ExecContext(ctx, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (p *PreparedStmt) Close() error {
	return p.stmt.Close()
}
