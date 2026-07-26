package sqldbs

// Not negates a condition: NOT (cond).
type Not struct {
	Cond Cond
}

func (n Not) BindRepr() (string, []any) {
	s, args := n.Cond.BindRepr()
	return "NOT (" + s + ")", args
}
