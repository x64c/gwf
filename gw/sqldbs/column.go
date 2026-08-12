package sqldbs

import "fmt"

// Column is a validated SQL identifier (e.g. "user.email").
//
// Its name is unexported, so only NewColumn can put one there. The zero value is
// still constructible and names nothing: a Column is a guarantee about the name
// it holds, not a guarantee that it holds one.
type Column struct {
	name string // unexported → cannot bypass validation
}

// Name returns the identifier string.
func (c Column) Name() string { return c.name }

// NewColumn validates the name and returns a Column carrying it.
//
// There is deliberately no panicking variant: a Column is either built inside
// NewTable, which reports the error, or by a caller naming a column of its
// own, which handles the error before passing it on. A panicking constructor
// would read as the convenient spelling and turn a bad identifier into a
// panicked request.
func NewColumn(name string) (Column, error) {
	if !IdentifierRegexp.MatchString(name) {
		return Column{}, fmt.Errorf("invalid SQL identifier: %q", name)
	}
	return Column{name: name}, nil
}
