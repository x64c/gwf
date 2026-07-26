package sqldbs

import "strings"

// Or combines conditions with OR: (c1 OR c2 OR ...).
type Or struct {
	Conds []Cond
}

func (o Or) BindRepr() (string, []any) {
	if len(o.Conds) == 0 {
		return "", nil
	}
	if len(o.Conds) == 1 {
		return o.Conds[0].BindRepr()
	}
	var b strings.Builder
	var args []any
	b.WriteByte('(')
	for i, c := range o.Conds {
		if i > 0 {
			b.WriteString(" OR ")
		}
		s, a := c.BindRepr()
		b.WriteString(s)
		args = append(args, a...)
	}
	b.WriteByte(')')
	return b.String(), args
}
