package sqldbs

import "context"

// The single-column-key conveniences. Most tables have a one-column key, and
// spelling PK{...} at every such call site says nothing the Table does not
// already know. Each function is a plain wrapper over its Executor method —
// the tuple form remains the one contract; these only spell its common case.
//
// id here is the model-form identity, which for a single-column key IS the
// key's bare value — the width-1 coincidence these wrappers exist to serve.
// The wrapper does the PK{} wrapping.
//
// They are package functions rather than interface methods so they are
// written once, not once per implementation, and the interface stays the
// minimal contract.

// SelectRowSinglePK is SelectRow for a single-column key: id is the value in
// the key column.
func SelectRowSinglePK(ctx context.Context, exec Executor, table *Table, id any, colNames ...string) (Row, error) {
	return exec.SelectRow(ctx, table, PK{id}, colNames...)
}

// UpdateRowSinglePK is UpdateRow for a single-column key: id is the value in
// the key column.
func UpdateRowSinglePK(ctx context.Context, exec Executor, table *Table, id any, columns []string, values []any) (int64, error) {
	return exec.UpdateRow(ctx, table, PK{id}, columns, values)
}

// DeleteRowSinglePK is DeleteRow for a single-column key: id is the value in
// the key column.
func DeleteRowSinglePK(ctx context.Context, exec Executor, table *Table, id any) (int64, error) {
	return exec.DeleteRow(ctx, table, PK{id})
}
