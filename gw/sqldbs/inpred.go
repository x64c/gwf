package sqldbs

import "strings"

// InPred is an IN predicate: column IN (values...).
type InPred struct {
	Column Column
	Values []any
}

func (p InPred) BindRepr() (string, []any) {
	var b strings.Builder
	b.WriteString(p.Column.Name())
	b.WriteString(" IN (")
	for i := range p.Values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('?')
	}
	b.WriteString(")")
	return b.String(), p.Values
}
