package sqldbs

import (
	"fmt"
	"regexp"
	"strings"
)

var IdentifierRegexp = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// ValidateIdentifier checks if a string is a safe SQL identifier.
func ValidateIdentifier(name string) error {
	if !IdentifierRegexp.MatchString(name) {
		return fmt.Errorf("invalid SQL identifier: %q", name)
	}
	return nil
}

// ValidateIdentifiers checks if all strings are safe SQL identifiers.
func ValidateIdentifiers(names []string) error {
	for _, name := range names {
		if !IdentifierRegexp.MatchString(name) {
			return fmt.Errorf("invalid SQL identifier: %q", name)
		}
	}
	return nil
}

// QuoteJoinIdentifiers quotes each identifier using the client's dialect and joins with ", ".
func QuoteJoinIdentifiers(c Client, columns []string) string {
	quoted := make([]string, len(columns))
	for i, col := range columns {
		quoted[i] = c.QuoteIdentifier(col)
	}
	return strings.Join(quoted, ", ")
}
