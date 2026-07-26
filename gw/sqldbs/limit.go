package sqldbs

import "fmt"

// LimitClause returns a LIMIT clause fragment, or empty string if limit <= 0.
func LimitClause(limit int) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf(" LIMIT %d", limit)
}
