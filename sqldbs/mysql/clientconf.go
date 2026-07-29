package mysql

// ClientConf holds shared MySQL server connection config.
type ClientConf struct {
	Host   string            `json:"host"`
	Port   int               `json:"port"`
	User   string            `json:"user"`
	PW     string            `json:"pw"`
	TZ     string            `json:"tz"`
	Params map[string]string `json:"params"` // extra DSN parameters — MySQL's own knobs, see below
	DSN    string            `json:"dsn"`    // To overwrite default DSN building — bypasses Params too
}

// Params are appended to the generated DSN verbatim, sorted by key so one conf
// always yields one DSN. They are this dialect's knobs, not the framework's:
// it takes no position on which belong in a given deployment, and adds none of
// its own beyond what the scan layer requires (parseTime, loc).
//
// Values are used AS WRITTEN — pre-escape what needs it, the way `tz` already
// is (`America%2FNew_York`).
//
// Two of them materially change SQL semantics. This driver used to set both
// silently; they now come from conf or not at all:
//
//	sql_mode=ANSI_QUOTES  — makes MySQL read "x" as an identifier (the SQL
//	                        standard, and what PostgreSQL does natively) instead
//	                        of a string literal. Raw SQL shared across dialects
//	                        relies on it to quote identifiers portably; without
//	                        it such a query silently returns the literal text
//	                        rather than the column — wrong data, no error.
//	multiStatements=true  — permits several statements per query, widening the
//	                        blast radius of any query-construction defect from
//	                        one statement to a batch. (Parameterised queries are
//	                        single-statement regardless — they are prepared.)
