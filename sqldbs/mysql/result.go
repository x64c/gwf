package mysql

import (
	"database/sql"

	"github.com/x64c/gwf/gw/sqldbs"
)

// Ensure mysql.Result implements sqldbs.Result interface
var _ sqldbs.Result = (*Result)(nil)

type Result struct {
	result sql.Result
}

func (r *Result) RowsAffected() (int64, error) {
	return r.result.RowsAffected()
}

func (r *Result) LastInsertId() (int64, error) {
	return r.result.LastInsertId()
}
