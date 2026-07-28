package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
)

// InsertModel inserts a model instance into its table.
// If AutoIncrement is false, PK column + GetID are included in the INSERT.
// If AutoIncrement is true, PK is excluded — use result.LastInsertId() for the assigned ID.
func InsertModel[
	M any,
	MP WritableIdentifiable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, model MP) (Result, error) {
	meta := model.TableMeta()
	fieldMap := model.FieldsToWrite()
	if meta.AutoIncrement && len(fieldMap) == 0 {
		return nil, fmt.Errorf("InsertModel: %q has no writable fields", meta.Name)
	}
	columns := make([]string, 0, len(fieldMap)+1)
	values := make([]any, 0, len(fieldMap)+1)
	if !meta.AutoIncrement {
		columns = append(columns, meta.PK)
		values = append(values, model.GetID())
	}
	for col, val := range fieldMap {
		columns = append(columns, col)
		values = append(values, val)
	}
	return exec.InsertRow(ctx, meta.Name, columns, values)
}

// InsertModelCollection inserts all items in a collection using a single multi-row INSERT.
// Column order is derived from the first item's FieldsToWrite map; all items must have the same columns.
func InsertModelCollection[
	M any,
	MP WritableIdentifiable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, items *coll.Collection[MP, ID]) (int64, error) {
	if items.Len() == 0 {
		return 0, nil
	}

	// Get columns from the first item
	first, _ := items.First()
	meta := first.TableMeta()
	firstMap := first.FieldsToWrite()
	if meta.AutoIncrement && len(firstMap) == 0 {
		return 0, fmt.Errorf("InsertModelCollection: %q has no writable fields", meta.Name)
	}

	columns := make([]string, 0, len(firstMap)+1)
	if !meta.AutoIncrement {
		columns = append(columns, meta.PK)
	}
	for col := range firstMap {
		columns = append(columns, col)
	}

	// Build row values for all items
	rowValues := make([][]any, 0, items.Len())
	items.ForEach(func(item MP) {
		fieldMap := item.FieldsToWrite()
		row := make([]any, len(columns))
		i := 0
		if !meta.AutoIncrement {
			row[0] = item.GetID()
			i = 1
		}
		for _, col := range columns[i:] {
			row[i] = fieldMap[col]
			i++
		}
		rowValues = append(rowValues, row)
	})

	return exec.InsertRows(ctx, meta.Name, columns, rowValues)
}

// UpdateModel updates a model instance in its table by PK.
// If updateColumns is nil, all columns from FieldsToWrite are updated.
// If updateColumns is provided, only those columns are updated.
func UpdateModel[
	M any,
	MP WritableIdentifiable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, model MP, updateColumns []string) (Result, error) {
	meta := model.TableMeta()
	fieldMap := model.FieldsToWrite()
	if len(fieldMap) == 0 {
		return nil, fmt.Errorf("UpdateModel: %q has no writable fields", meta.Name)
	}
	var columns []string
	var values []any
	if len(updateColumns) == 0 {
		columns = make([]string, 0, len(fieldMap))
		values = make([]any, 0, len(fieldMap))
		for col, val := range fieldMap {
			columns = append(columns, col)
			values = append(values, val)
		}
	} else {
		columns = updateColumns
		values = make([]any, len(updateColumns))
		for i, col := range updateColumns {
			// Allow-list, not pattern-match: an update column must be one the
			// model declares writable. That keeps caller-supplied strings from
			// reaching SQL as identifiers at all, and catches the silent
			// alternative — an unknown key would otherwise write NULL.
			val, ok := fieldMap[col]
			if !ok {
				return nil, fmt.Errorf("UpdateModel: %q is not a writable field of %q", col, meta.Name)
			}
			values[i] = val
		}
	}
	return exec.UpdateRow(ctx, meta.Name, meta.PK, model.GetID(), columns, values)
}

// UpdateModelCollection updates all items in a collection by looping UpdateRow per item.
// No batch SQL exists for multi-row UPDATE — each item is a separate UPDATE statement.
// For the param `exec`, pass Tx for atomic all-or-nothing, or DB for individual auto-committed updates.
// Scale matters: for large collections (e.g. 10K+ depending on DBMS), consider chunking.
// If updateColumns is nil, all columns from FieldsToWrite are updated.
func UpdateModelCollection[
	M any,
	MP WritableIdentifiable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, items *coll.Collection[MP, ID], updateColumns []string) (int64, error) {
	if items.Len() == 0 {
		return 0, nil
	}

	// Get columns once from the first item
	first, _ := items.First()
	meta := first.TableMeta()
	if len(first.FieldsToWrite()) == 0 {
		return 0, fmt.Errorf("UpdateModelCollection: %q has no writable fields", meta.Name)
	}
	var columns []string
	if len(updateColumns) > 0 {
		// Allow-list against what the model declares writable — same reason as
		// UpdateModel: caller-supplied strings must never reach SQL as raw
		// identifiers, and an unknown key would otherwise write NULL per item.
		firstMap := first.FieldsToWrite()
		for _, col := range updateColumns {
			if _, ok := firstMap[col]; !ok {
				return 0, fmt.Errorf("UpdateModelCollection: %q is not a writable field of %q", col, meta.Name)
			}
		}
		columns = updateColumns
	} else {
		firstMap := first.FieldsToWrite()
		columns = make([]string, 0, len(firstMap))
		for col := range firstMap {
			columns = append(columns, col)
		}
	}

	// Loop: build values per item, reuse columns
	var totalAffected int64
	var firstErr error
	items.ForEach(func(item MP) {
		if firstErr != nil {
			return
		}
		fieldMap := item.FieldsToWrite()
		values := make([]any, len(columns))
		for i, col := range columns {
			values[i] = fieldMap[col]
		}
		result, err := exec.UpdateRow(ctx, meta.Name, meta.PK, item.GetID(), columns, values)
		if err != nil {
			firstErr = err
			return
		}
		n, _ := result.RowsAffected()
		totalAffected += n
	})
	return totalAffected, firstErr
}

// DeleteModel deletes a model instance from its table by PK.
func DeleteModel[
	M any,
	MP Deletable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, model MP) (Result, error) {
	meta := model.TableMeta()
	return exec.DeleteRow(ctx, meta.Name, meta.PK, model.GetID())
}

// DeleteModelCollection deletes all items in a collection using a single DELETE WHERE pk IN (...).
func DeleteModelCollection[
	M any,
	MP Deletable[M, ID],
	ID comparable,
](ctx context.Context, exec Executor, items *coll.Collection[MP, ID]) (int64, error) {
	if items.Len() == 0 {
		return 0, nil
	}

	first, _ := items.First()
	meta := first.TableMeta()
	pkCol, _ := NewColumn(meta.PK)

	ids := make([]any, 0, items.Len())
	items.ForEach(func(item MP) {
		ids = append(ids, item.GetID())
	})

	return exec.DeleteRows(ctx, meta.Name, InPred{Column: pkCol, Values: ids})
}
