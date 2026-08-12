package sqldbs

import (
	"context"
	"log"

	"github.com/x64c/gwf/gw/coll"
)

// The DBModel raw-query functions: each runs the statement as given and
// scans through the DBModel scan functions.

// RawQueryDBModel queries one row into a fresh instance and returns it. A
// missing row arrives as ErrNoRows.
func RawQueryDBModel[M any, MP DBModel[M]](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any,
) (MP, error) {
	row := db.QueryRowRaw(ctx, rawSQLStmt, args...)
	return ScanRowToDBModel[M, MP](row)
}

// RawQueryDBModels queries rows into fresh instances and returns them in row
// order.
func RawQueryDBModels[M any, MP DBModel[M]](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any,
) ([]MP, error) {
	rows, err := db.QueryRowsRaw(ctx, rawSQLStmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("rows.Close() failed: %v", err)
		}
	}()
	return ScanRowsToDBModels[M, MP](rows)
}

// RawQueryDBModelMap queries rows into fresh instances keyed by GetID.
func RawQueryDBModelMap[M any, MP IdentifiableDBModel[M, ID], ID comparable](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any,
) (map[ID]MP, error) {
	rows, err := db.QueryRowsRaw(ctx, rawSQLStmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("rows.Close() failed: %v", err)
		}
	}()
	return ScanRowsToDBModelMap[M, MP, ID](rows)
}

// RawQueryDBModelCollection queries rows into fresh instances collected in
// row order.
func RawQueryDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any,
) (*coll.Collection[MP, ID], error) {
	rows, err := db.QueryRowsRaw(ctx, rawSQLStmt, args...)
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
