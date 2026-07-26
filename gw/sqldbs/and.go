package sqldbs

import "strings"

// And combines conditions with AND: (c1 AND c2 AND ...).
type And struct {
	Conds []Cond
}

func (a And) BindRepr() (string, []any) {
	if len(a.Conds) == 0 {
		return "", nil
	}
	if len(a.Conds) == 1 {
		return a.Conds[0].BindRepr()
	}
	var b strings.Builder
	var args []any
	b.WriteByte('(')
	for i, c := range a.Conds {
		if i > 0 {
			b.WriteString(" AND ")
		}
		s, a := c.BindRepr()
		b.WriteString(s)
		args = append(args, a...)
	}
	b.WriteByte(')')
	return b.String(), args
}
