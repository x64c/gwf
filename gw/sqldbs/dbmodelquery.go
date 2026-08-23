package sqldbs

import (
	"context"
	"fmt"
	"log"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
)

// The DBModel query functions. The SELECT is derived from the model: its
// Table names the table, its ColumnFieldBindings name the columns in scan
// order.

// SelectBaseOf builds "SELECT <bound columns, binding order> FROM <table>"
// for the model — the derivation FetchDBModel makes — with identifiers quoted
// by the client. The bound columns are Choose-validated against the Table.
func SelectBaseOf[M any, MP DBModel[M]](c Client) (string, error) {
	model := MP(new(M))
	bindings := model.ColumnFieldBindings()
	colNames := make([]string, len(bindings))
	for i, b := range bindings {
		colNames[i] = b.Column
	}
	tbl := model.Table()
	cols, err := tbl.Columns().Choose(colNames...)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT %s FROM %s", QuoteJoinIdentifiers(c, cols.Names()), c.QuoteIdentifier(tbl.Name())), nil
}

// QueryDBModelFirst queries a single instance using QueryOpts with LIMIT 1.
// Returns the instance or ErrNoRows if none matched.
// QueryOpts.Limit must be 0 (omitted) or 1; greater than 1 returns an error.
func QueryDBModelFirst[M any, MP DBModel[M]](
	ctx context.Context,
	db DB,
	queryOpts QueryOpts,
) (MP, error) {
	var zero MP
	if queryOpts.Limit > 1 {
		return zero, errs.SQLDB.WithDetail("QueryDBModelFirst does not accept Limit greater than 1")
	}
	sqlSelectBase, err := SelectBaseOf[M, MP](db.Client())
	if err != nil {
		return zero, fmt.Errorf("QueryDBModelFirst: %w", err)
	}
	whereSQL, args := WhereClause{queryOpts.WhereCond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys) + LimitClause(1)
	return RawQueryDBModel[M, MP](ctx, db, sqlStmt, args...)
}

// QueryDBModelCollection queries instances into a collection using QueryOpts.
func QueryDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](
	ctx context.Context,
	db DB,
	queryOpts QueryOpts,
) (*coll.Collection[MP, ID], error) {
	sqlSelectBase, err := SelectBaseOf[M, MP](db.Client())
	if err != nil {
		return nil, fmt.Errorf("QueryDBModelCollection: %w", err)
	}
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
	column Column,
	values []V,
	orderBys ...OrderBy,
) (*coll.Collection[MP, ID], error) {
	if len(values) == 0 {
		return nil, errs.SQLDB.WithDetail("QueryDBModelCollectionByColumn requires at least one value")
	}
	dbClient := db.Client()
	sqlSelectBase, err := SelectBaseOf[M, MP](dbClient)
	if err != nil {
		return nil, fmt.Errorf("QueryDBModelCollectionByColumn: %w", err)
	}
	var rows Rows
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
