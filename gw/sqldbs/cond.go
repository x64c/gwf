package sqldbs

// Cond represents a SQL condition expression.
// Implemented by predicates (BinPred, InPred, NullPred, NotNullPred)
// and logical operators (Not, And, Or).
// BindRepr returns a SQL fragment with generic ? placeholders and bind args.
// Dialect-specific placeholder translation (e.g. ? → $N) is handled by the consumer.
type Cond interface {
	BindRepr() (string, []any)
}
