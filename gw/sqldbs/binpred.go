package sqldbs

// BinPred is a binary predicate: column OP value.
type BinPred struct {
	Column Column
	Op     BinOp
	Value  any
}

func (p BinPred) BindRepr() (string, []any) {
	return p.Column.Name() + " " + p.Op.op + " ?", []any{p.Value}
}
