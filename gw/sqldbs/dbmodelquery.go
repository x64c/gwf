package sqldbs

import (
	"context"
	"fmt"
	"log"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
)

// The DBModel query functions. sqlSelectBase must be clean of WHERE and
// bindings, and its column order must match the model's binding order.

// QueryDBModelFirst queries a single instance using QueryOpts with LIMIT 1.
// Returns the instance or ErrNoRows if none matched.
// QueryOpts.Limit must be 0 (omitted) or 1; greater than 1 returns an error.
func QueryDBModelFirst[M any, MP DBModel[M]](
	ctx context.Context,
	db DB,
	sqlSelectBase string,
	queryOpts QueryOpts,
) (MP, error) {
	if queryOpts.Limit > 1 {
		var zero MP
		return zero, errs.SQLDB.WithDetail("QueryDBModelFirst does not accept Limit greater than 1")
	}
	whereSQL, args := WhereClause{queryOpts.WhereCond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys) + LimitClause(1)
	return RawQueryDBModel[M, MP](ctx, db, sqlStmt, args...)
}

// QueryDBModelCollection queries instances into a collection using QueryOpts.
func QueryDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](
	ctx context.Context,
	db DB,
	sqlSelectBase string,
	queryOpts QueryOpts,
) (*coll.Collection[MP, ID], error) {
	whereSQL, args := WhereClause{queryOpts.WhereCond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys) + LimitClause(queryOpts.Limit)
	return RawQueryDBModelCollection[M, MP, ID](ctx, db, sqlStmt, args...)
}

// QueryDBModelCollectionByColumn queries instances where a column matches one
// or more values: WHERE column = ? for a single value, WHERE column IN (?, ...)
// for several.
func QueryDBModelCollectionByColumn[M any, MP IdentifiableDBModel[M, ID], ID comparable, V any](
	ctx context.Context,
	db DB,
	sqlSelectBase string,
	column Column,
	values []V,
	orderBys ...OrderBy,
) (*coll.Collection[MP, ID], error) {
	if len(values) == 0 {
		return nil, errs.SQLDB.WithDetail("QueryDBModelCollectionByColumn requires at least one value")
	}
	dbClient := db.Client()
	var (
		rows Rows
		err  error
	)
	if len(values) == 1 {
		whereClause := fmt.Sprintf(" WHERE %s = %s", column.Name(), dbClient.FirstPlaceholder())
		sqlStmt := sqlSelectBase + whereClause + OrderByClause(orderBys)
		rows, err = db.SelectRowsRaw(ctx, sqlStmt, values[0])
	} else {
		whereClause := fmt.Sprintf(" WHERE %s IN (%s)", column.Name(), dbClient.InPlaceholders(1, len(values)))
		sqlStmt := sqlSelectBase + whereClause + OrderByClause(orderBys)
		valuesAsAny := make([]any, len(values))
		for i, v := range values {
			valuesAsAny[i] = v
		}
		rows, err = db.SelectRowsRaw(ctx, sqlStmt, valuesAsAny...)
	}
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("rows.Close() failed: %v", err)
		}
	}()
	return ScanRowsToDBModelCollection[M, MP, ID](rows)
}
