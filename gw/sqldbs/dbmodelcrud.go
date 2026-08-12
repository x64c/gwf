package sqldbs

import (
	"context"
	"fmt"
)

// The DBModel per-row operations: each takes the model whose bindings drive
// the statement.

// HydrateDBModel completes the instance it is given: key fields in, the rest
// filled from the row they name. The key is read from the KEY BINDINGS — the
// bindings whose columns are the table's key columns, in the table's key
// order — and their pointers are bound directly into the statement, so the
// key never leaves the model and nothing is dereferenced here.
//
// The non-key columns are selected by name and scanned into their own
// bindings' pointers. A table whose every column is key degenerates to
// selecting the key back into its own pointers — the same values rewritten,
// existence proven the same way: a missing row arrives as ErrNoRows from
// Scan.
func HydrateDBModel[M any, MP DBModel[M]](ctx context.Context, exec Executor, model MP) error {
	tbl := model.Table()
	keyBs, nonKeyBs, err := splitBindingsByKey(tbl, model.ColumnFieldBindings())
	if err != nil {
		return fmt.Errorf("HydrateDBModel: %w", err)
	}

	pkValue := pkValueFromBindings(keyBs)

	selBs := nonKeyBs
	if len(selBs) == 0 {
		selBs = keyBs // pure-key table — see the doc
	}
	colNames := make([]string, len(selBs))
	dests := make([]any, len(selBs))
	for i, b := range selBs {
		colNames[i] = b.Column
		dests[i] = b.FieldPtr
	}

	row, err := exec.SelectRow(ctx, tbl, pkValue, colNames...)
	if err != nil {
		return fmt.Errorf("HydrateDBModel: %w", err)
	}
	return row.Scan(dests...)
}

// FetchDBModel reads the row pkValue names into a fresh instance and returns
// it — the allocate-and-return sibling of HydrateDBModel, for a caller
// holding an identity rather than a model. pkValue is the database's identity
// form: write PK{...}.
//
// Every bound column is selected, the key included, so the instance comes
// back complete — the database repeating the key the caller stated. Type
// parameters cannot be inferred here (the model appears only in the return),
// so call sites name them, as the query family always has:
//
//	m, err := FetchDBModel[DevMcp, *DevMcp](ctx, db, PK{7, "abc"})
func FetchDBModel[M any, MP DBModel[M]](ctx context.Context, exec Executor, pkValue PK) (MP, error) {
	var zero MP
	model := MP(new(M))
	bindings := model.ColumnFieldBindings()
	colNames := make([]string, len(bindings))
	dests := make([]any, len(bindings))
	for i, b := range bindings {
		colNames[i] = b.Column
		dests[i] = b.FieldPtr
	}
	row, err := exec.SelectRow(ctx, model.Table(), pkValue, colNames...)
	if err != nil {
		return zero, fmt.Errorf("FetchDBModel: %w", err)
	}
	if err := row.Scan(dests...); err != nil {
		return zero, err
	}
	return model, nil
}

// InsertDBModel inserts the model AS-IS: every binding becomes a column of
// the INSERT — key included — with values exactly as the fields hold them.
// Nothing is generated, omitted, or inspected: there is no reserved zero
// here, so a key the caller forgot to set goes to the database as zeros, and
// the database judges it. The database generating the key is a different
// call: InsertDBModelAutoIncrementingPK.
//
// The field pointers are the bind args; the drivers dereference them.
func InsertDBModel[M any, MP DBModel[M]](ctx context.Context, exec Executor, model MP) error {
	bindings := model.ColumnFieldBindings()
	columns := make([]string, len(bindings))
	values := make([]any, len(bindings))
	for i, b := range bindings {
		columns[i] = b.Column
		values[i] = b.FieldPtr
	}
	if err := exec.InsertRow(ctx, model.Table(), columns, values); err != nil {
		return fmt.Errorf("InsertDBModel: %w", err)
	}
	return nil
}

// InsertDBModelAutoIncrementingPK inserts the model WITHOUT its key bindings and
// returns the key the database generated. The table must declare an
// auto-increment key; the drivers enforce that and everything else
// InsertRowAutoIncrementingPK promises.
//
// A model already carrying a key is REFUSED — a caller who chose the key is
// not asking for one to be generated; that is InsertDBModel. The check is a
// zero test on GetID, which is why this function demands IdentifiableDBModel
// where its siblings take DBModel — and the test is principled here, not the
// reserved-zero convention the other operations shed: a generated key is
// never 0 by this operation's own contract, so zero IS "no key yet" in this
// domain.
//
// The model's key field is NOT set from the returned value — pushing it back
// through the binding would take a runtime type assertion, which this package
// does not do. The caller assigns it.
func InsertDBModelAutoIncrementingPK[M any, MP IdentifiableDBModel[M, ID], ID comparable](ctx context.Context, exec Executor, model MP) (int64, error) {
	var zeroID ID
	if model.GetID() != zeroID {
		return 0, fmt.Errorf("InsertDBModelAutoIncrementingPK: %q already carries a key, so nothing is generated here — use InsertDBModel", model.Table().Name())
	}
	tbl := model.Table()
	_, nonKeyBs, err := splitBindingsByKey(tbl, model.ColumnFieldBindings())
	if err != nil {
		return 0, fmt.Errorf("InsertDBModelAutoIncrementingPK: %w", err)
	}
	columns := make([]string, len(nonKeyBs))
	values := make([]any, len(nonKeyBs))
	for i, b := range nonKeyBs {
		columns[i] = b.Column
		values[i] = b.FieldPtr
	}
	pk, err := exec.InsertRowAutoIncrementingPK(ctx, tbl, columns, values)
	if err != nil {
		return 0, fmt.Errorf("InsertDBModelAutoIncrementingPK: %w", err)
	}
	return pk, nil
}

// UpdateDBModel updates the row carrying the model's key and reports how many
// rows it matched — 0 meaning there is no such row. The key bindings locate
// the row; the non-key bindings are written as-is. updateColumns narrows the
// write to the named columns — it filters what the model already knows, so it
// is variadic and empty means all non-key bindings. A name that is not among
// the non-key bindings — the key columns included — is refused.
func UpdateDBModel[M any, MP DBModel[M]](ctx context.Context, exec Executor, model MP, updateColumns ...string) (int64, error) {
	tbl := model.Table()
	keyBs, nonKeyBs, err := splitBindingsByKey(tbl, model.ColumnFieldBindings())
	if err != nil {
		return 0, fmt.Errorf("UpdateDBModel: %w", err)
	}

	writeBs := nonKeyBs
	if len(updateColumns) > 0 {
		writeBs = make([]ColumnFieldBinding, 0, len(updateColumns))
		for _, name := range updateColumns {
			found := false
			for _, b := range nonKeyBs {
				if b.Column == name {
					writeBs = append(writeBs, b)
					found = true
					break
				}
			}
			if !found {
				return 0, fmt.Errorf("UpdateDBModel: table %q: %q is not among the model's non-key bindings", tbl.Name(), name)
			}
		}
	}
	if len(writeBs) == 0 {
		return 0, fmt.Errorf("UpdateDBModel: table %q has no non-key bindings — nothing to update", tbl.Name())
	}

	columns := make([]string, len(writeBs))
	values := make([]any, len(writeBs))
	for i, b := range writeBs {
		columns[i] = b.Column
		values[i] = b.FieldPtr
	}
	n, err := exec.UpdateRow(ctx, tbl, pkValueFromBindings(keyBs), columns, values)
	if err != nil {
		return 0, fmt.Errorf("UpdateDBModel: %w", err)
	}
	return n, nil
}

// DeleteDBModel deletes the row carrying the model's key and reports how many
// rows it matched — 0 meaning there was no such row. Only the key bindings
// take part; every other field is ignored.
func DeleteDBModel[M any, MP DBModel[M]](ctx context.Context, exec Executor, model MP) (int64, error) {
	tbl := model.Table()
	keyBs, _, err := splitBindingsByKey(tbl, model.ColumnFieldBindings())
	if err != nil {
		return 0, fmt.Errorf("DeleteDBModel: %w", err)
	}
	n, err := exec.DeleteRow(ctx, tbl, pkValueFromBindings(keyBs))
	if err != nil {
		return 0, fmt.Errorf("DeleteDBModel: %w", err)
	}
	return n, nil
}

// pkValueFromBindings renders key bindings as a PK: the pointers themselves,
// in the key order splitBindingsByKey established — the drivers dereference.
func pkValueFromBindings(keyBs []ColumnFieldBinding) PK {
	pkValue := make(PK, len(keyBs))
	for i, b := range keyBs {
		pkValue[i] = b.FieldPtr
	}
	return pkValue
}

// splitBindingsByKey separates a model's bindings into the key bindings —
// ordered by the TABLE's key order, which is what a PK's positions mean — and
// the rest, in their declared order. A model whose bindings miss a key
// column, or bind any column twice, is refused.
func splitBindingsByKey(tbl *Table, bindings []ColumnFieldBinding) (key []ColumnFieldBinding, nonKey []ColumnFieldBinding, err error) {
	byCol := make(map[string]ColumnFieldBinding, len(bindings))
	for _, b := range bindings {
		if _, dup := byCol[b.Column]; dup {
			return nil, nil, fmt.Errorf("table %q: column %q bound twice", tbl.Name(), b.Column)
		}
		byCol[b.Column] = b
	}
	pkNames := tbl.PKColumns().Names()
	key = make([]ColumnFieldBinding, 0, len(pkNames))
	for _, name := range pkNames {
		b, ok := byCol[name]
		if !ok {
			return nil, nil, fmt.Errorf("table %q: no binding for key column %q", tbl.Name(), name)
		}
		key = append(key, b)
		delete(byCol, name)
	}
	nonKey = make([]ColumnFieldBinding, 0, len(byCol))
	for _, b := range bindings {
		if _, ok := byCol[b.Column]; ok {
			nonKey = append(nonKey, b)
		}
	}
	return key, nonKey, nil
}
