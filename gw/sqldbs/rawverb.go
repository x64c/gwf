package sqldbs

import (
	"fmt"
	"strings"
)

// CheckRawVerb guards a caller-written raw statement: the query must start
// with the verb its method promises (leading whitespace ignored, case
// insensitive). method names the caller in the error.
func CheckRawVerb(method, verb, query string) error {
	trimmed := strings.TrimSpace(query)
	if !strings.HasPrefix(strings.ToUpper(trimmed), verb) {
		return fmt.Errorf("%s: query must start with %s", method, verb)
	}
	return nil
}
