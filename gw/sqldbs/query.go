package sqldbs

import (
	"context"
	"fmt"
	"log"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
)

// QueryFirst queries a single model using QueryOpts with LIMIT 1.
// Returns the item or ErrNoRows if not found.
// QueryOpts.Limit must be 0 (omitted) or 1; greater than 1 returns an error.
func QueryFirst[
	M any, // Model struct
	MP Scannable[M], // *Model implementing Scannable[M]
](
	ctx context.Context,
	db DB,
	sqlSelectBase string, // must be clean from WHERE and bindings
	queryOpts QueryOpts,
) (*M, error) {
	if queryOpts.Limit > 1 {
		return nil, errs.SQLDB.WithDetail("QueryFirst does not accept Limit greater than 1")
	}
	whereSQL, args := WhereClause{queryOpts.WhereCond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys) + LimitClause(1)
	return RawQueryItem[M, MP](ctx, db, sqlStmt, args...)
}

// QueryCollection queries models into a collection using QueryOpts.
// Builds a standalone clause from QueryOpts, starting with WHERE if any conditions exist.
func QueryCollection[
	M any, // Model struct
	MP ScannableIdentifiable[M, ID], // *Model implementing ScannableIdentifiable[M, ID]
	ID comparable,
](
	ctx context.Context,
	db DB,
	sqlSelectBase string, // must be clean from WHERE and bindings
	queryOpts QueryOpts,
) (*coll.Collection[MP, ID], error) {
	whereSQL, args := WhereClause{queryOpts.WhereCond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys) + LimitClause(queryOpts.Limit)
	return RawQueryCollection[M, MP, ID](ctx, db, sqlStmt, args...)
}

// QueryCollectionByColumn queries models where a column matches one or more values.
// Uses WHERE column = ? for single value, WHERE column IN (?, ...) for multiple.
// Returns a collection of scanned models.
func QueryCollectionByColumn[
	M any, // Model struct
	MP ScannableIdentifiable[M, ID], // *Model implementing ScannableIdentifiable[M, ID]
	ID comparable,
	V any,
](
	ctx context.Context,
	db DB,
	sqlSelectBase string, // must be clean from WHERE and bindings
	column Column,
	values []V,
	orderBys ...OrderBy,
) (*coll.Collection[MP, ID], error) {
	if len(values) == 0 {
		return nil, errs.SQLDB.WithDetail("QueryCollectionByColumn requires at least one value")
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
	return ScanRowsToCollection[M, MP, ID](rows)
}
