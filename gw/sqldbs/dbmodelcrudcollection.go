package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
)

// The DBModel collection operations — bulk forms of the per-row family.

// InsertDBModelCollection inserts every instance with a single multi-row
// INSERT, each written AS-IS like InsertDBModel: every binding a column, key
// included, nothing generated or inspected. Columns and their order come
// from the first instance's bindings; the instances share a type, and a
// type's bindings shape does not vary between instances, so every row
// aligns. An empty collection inserts nothing and reports 0.
//
// There is deliberately no AutoIncrementing collection variant: mysql's
// LastInsertId can return only one generated key per statement, so a
// per-row-keys contract cannot be kept across engines — a caller generating
// keys per row loops InsertDBModelAutoIncrementingPK.
func InsertDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](ctx context.Context, exec Executor, modelCollection *coll.Collection[MP, ID]) (int64, error) {
	if modelCollection.Len() == 0 {
		return 0, nil
	}
	first, _ := modelCollection.First()
	tbl := first.Table()
	firstBs := first.ColumnFieldBindings()
	columns := make([]string, len(firstBs))
	for i, b := range firstBs {
		columns[i] = b.Column
	}
	rowValues := make([][]any, 0, modelCollection.Len())
	modelCollection.ForEach(func(m MP) {
		bs := m.ColumnFieldBindings()
		row := make([]any, len(bs))
		for i, b := range bs {
			row[i] = b.FieldPtr
		}
		rowValues = append(rowValues, row)
	})
	n, err := exec.InsertRows(ctx, tbl, columns, rowValues)
	if err != nil {
		return 0, fmt.Errorf("InsertDBModelCollection: %w", err)
	}
	return n, nil
}

// UpdateDBModelCollection updates every instance by its key — one UPDATE per
// instance, since SQL has no multi-row UPDATE — and reports the total rows
// matched. The first error stops the loop; pass a Tx for all-or-nothing, or
// a DB for individually auto-committed updates. updateColumns narrows every
// instance's write to the named columns, with UpdateDBModel's rules. An
// empty collection updates nothing and reports 0.
//
// Scale matters: for large collections, consider chunking.
func UpdateDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](ctx context.Context, exec Executor, modelCollection *coll.Collection[MP, ID], updateColumns ...string) (int64, error) {
	var total int64
	var firstErr error
	modelCollection.ForEach(func(m MP) {
		if firstErr != nil {
			return
		}
		n, err := UpdateDBModel(ctx, exec, m, updateColumns...)
		if err != nil {
			firstErr = fmt.Errorf("UpdateDBModelCollection: %w", err)
			return
		}
		total += n
	})
	return total, firstErr
}

// DeleteDBModelCollection deletes every instance's row with a single
// DELETE WHERE pk IN (...) and reports the rows matched. Single-column keys
// only: a composite key needs row-value syntax our Cond cannot express yet.
// For a single-column key the model form IS the key's bare value, so the
// instances' GetID values are the list. An empty collection deletes nothing
// and reports 0.
func DeleteDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](ctx context.Context, exec Executor, modelCollection *coll.Collection[MP, ID]) (int64, error) {
	if modelCollection.Len() == 0 {
		return 0, nil
	}
	first, _ := modelCollection.First()
	tbl := first.Table()
	pkCol, err := tbl.SinglePKColumn()
	if err != nil {
		return 0, fmt.Errorf("DeleteDBModelCollection: %w", err)
	}
	n, err := exec.DeleteRows(ctx, tbl, InPred{Column: pkCol, Values: modelCollection.IDsAsAny()})
	if err != nil {
		return 0, fmt.Errorf("DeleteDBModelCollection: %w", err)
	}
	return n, nil
}
