package sqldbs

// QueryOpts holds optional query clauses for query functions that need
// WHERE conditions, ORDER BY, and LIMIT in a single parameter.
type QueryOpts struct {
	WhereCond Cond
	OrderBys  []OrderBy
	Limit     int // 0 = no limit
}
