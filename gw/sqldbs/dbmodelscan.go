package sqldbs

import (
	"fmt"

	"github.com/x64c/gwf/gw/coll"
)

// The DBModel scan functions.
// Destinations come from ColumnFieldBindings() on a fresh instance, in
// binding order: the statement's column order must match it — the same
// positional discipline the old family required of select.sql, now anchored
// to the one artifact that also names the columns.

// scanDests renders bindings as scan destinations: the field pointers, in
// binding order.
func scanDests(bindings []ColumnFieldBinding) []any {
	dests := make([]any, len(bindings))
	for i, b := range bindings {
		dests[i] = b.FieldPtr
	}
	return dests
}

// ScanRowToDBModel scans one row into a fresh instance and returns it. A
// missing row arrives as ErrNoRows from Scan.
func ScanRowToDBModel[M any, MP DBModel[M]](row Row) (MP, error) {
	model := MP(new(M))
	if err := row.Scan(scanDests(model.ColumnFieldBindings())...); err != nil {
		var zero MP
		return zero, err
	}
	return model, nil
}

// ScanRowsToDBModels scans every row into its own fresh instance and returns
// them in row order.
func ScanRowsToDBModels[M any, MP DBModel[M]](rows Rows) ([]MP, error) {
	var models []MP
	for rows.Next() {
		model := MP(new(M))
		if err := rows.Scan(scanDests(model.ColumnFieldBindings())...); err != nil {
			return nil, fmt.Errorf("ScanRowsToDBModels: %w", err)
		}
		models = append(models, model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ScanRowsToDBModels: %w", err)
	}
	return models, nil
}

// ScanRowsToDBModelMap scans every row into its own fresh instance, keyed by
// GetID. Row order is lost; a repeated ID keeps the last row.
func ScanRowsToDBModelMap[M any, MP IdentifiableDBModel[M, ID], ID comparable](rows Rows) (map[ID]MP, error) {
	models := map[ID]MP{}
	for rows.Next() {
		model := MP(new(M))
		if err := rows.Scan(scanDests(model.ColumnFieldBindings())...); err != nil {
			return nil, fmt.Errorf("ScanRowsToDBModelMap: %w", err)
		}
		models[model.GetID()] = model
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ScanRowsToDBModelMap: %w", err)
	}
	return models, nil
}

// ScanRowsToDBModelCollection scans every row into its own fresh instance and
// collects them in row order.
func ScanRowsToDBModelCollection[M any, MP IdentifiableDBModel[M, ID], ID comparable](rows Rows) (*coll.Collection[MP, ID], error) {
	c := coll.NewEmptyOrderedCollection[MP, ID]()
	for rows.Next() {
		model := MP(new(M))
		if err := rows.Scan(scanDests(model.ColumnFieldBindings())...); err != nil {
			return nil, fmt.Errorf("ScanRowsToDBModelCollection: %w", err)
		}
		c.Add(model)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ScanRowsToDBModelCollection: %w", err)
	}
	return c, nil
}
