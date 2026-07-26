package sqldbs

// NullPred is a null check predicate: column IS NULL.
type NullPred struct {
	Column Column
}

func (p NullPred) BindRepr() (string, []any) {
	return p.Column.Name() + " IS NULL", nil
}
