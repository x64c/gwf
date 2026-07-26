package sqldbs

import (
	"strconv"
	"strings"
)

// ReplaceStaticPlaceholders converts `?` placeholders to numbered placeholders
// (e.g. $1, $2 for PostgreSQL). Dynamic placeholders `??` are left untouched.
func ReplaceStaticPlaceholders(sql string, prefix byte) string {
	if prefix == '?' || prefix == 0 {
		return sql
	}
	var builder strings.Builder
	builder.Grow(len(sql) + 8)
	cnt := 1
	i := 0
	for i < len(sql) {
		if sql[i] == '?' {
			// Do Not Touch Dynamic Placeholders '??'
			if i+1 < len(sql) && sql[i+1] == '?' {
				builder.WriteByte('?')
				builder.WriteByte('?')
				i += 2
				continue
			}
			builder.WriteByte(prefix)
			builder.WriteString(strconv.Itoa(cnt))
			cnt++
		} else {
			builder.WriteByte(sql[i])
		}
		i++
	}
	return builder.String()
}
