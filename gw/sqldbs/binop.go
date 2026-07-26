package sqldbs

// BinOp is a binary operator symbol used in BinPred.
type BinOp struct {
	op string
}

var (
	OpEq      = BinOp{"="}
	OpNeq     = BinOp{"!="}
	OpGt      = BinOp{">"}
	OpLt      = BinOp{"<"}
	OpGtEq    = BinOp{">="}
	OpLtEq    = BinOp{"<="}
	OpLike    = BinOp{"LIKE"}
	OpNotLike = BinOp{"NOT LIKE"}
)
