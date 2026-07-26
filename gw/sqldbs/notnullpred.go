package sqldbs

// NotNullPred is a not-null check predicate: column IS NOT NULL.
type NotNullPred struct {
	Column Column
}

func (p NotNullPred) BindRepr() (string, []any) {
	return p.Column.Name() + " IS NOT NULL", nil
}
