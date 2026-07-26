package sqldbs

import "strings"

// NotInPred is a NOT IN predicate: column NOT IN (values...).
type NotInPred struct {
	Column Column
	Values []any
}

func (p NotInPred) BindRepr() (string, []any) {
	var b strings.Builder
	b.WriteString(p.Column.Name())
	b.WriteString(" NOT IN (")
	for i := range p.Values {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('?')
	}
	b.WriteString(")")
	return b.String(), p.Values
}
