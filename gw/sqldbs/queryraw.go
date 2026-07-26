package sqldbs

import (
	"context"
	"log"

	"github.com/x64c/gwf/gw/coll"
)

func RawQueryItem[
	M any, // Model struct
	MP Scannable[M], // *Model Implementing Scannable[M]
](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any, // variadic
) (*M, error) { // Returns the Pointer to the Newly Created Item
	row := db.QueryRowRaw(ctx, rawSQLStmt, args...)
	return ScanRowToItem[M, MP](row)
}

func RawQueryItems[
	M any, // Model struct
	MP Scannable[M], // *Model Implementing Scannable[M]
](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any, // variadic
) ([]*M, error) { // Returns a Slice of Model-Pointers
	rows, err := db.QueryRowsRaw(ctx, rawSQLStmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("rows.Close() failed: %v", err)
		}
	}()
	return ScanRowsToItems[M, MP](rows)
}

// RawQueryMap queries items using rawSQLStmt and scan rows to a map[id]item
func RawQueryMap[
	M any, // Model struct
	MP ScannableIdentifiable[M, ID], // *Model Implementing ScannableIdentifiable[M, ID]
	ID comparable,
](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any, // variadic
) (map[ID]*M, error) { // Returns a ItemsMap of ID to Model-Pointers
	rows, err := db.QueryRowsRaw(ctx, rawSQLStmt, args...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("rows.Close() failed: %v", err)
		}
	}()
	return ScanRowsToMap[M, MP, ID](rows)
}

// RawQueryCollection queries items using rawSQLStmt and scan rows to a collection
func RawQueryCollection[
	M any, // Model struct
	MP ScannableIdentifiable[M, ID], // *Model implementing ScannableIdentifiable[M, ID]
	ID comparable,
](
	ctx context.Context,
	db DB,
	rawSQLStmt string,
	args ...any, // variadic
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
	return ScanRowsToCollection[M, MP, ID](rows)
}
