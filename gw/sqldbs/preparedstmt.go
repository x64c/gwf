package sqldbs

import "context"

// PreparedStmt is a statement parsed once and then run repeatedly with different
// arguments.
//
// The caller owns it and MUST Close it. What Close releases is the
// implementation's business — it may be nothing at all — but the obligation
// stands either way, because an implementation that does hold something has no
// other moment to let it go.
//
// It is safe for concurrent use. An implementation must not bind the statement
// to one connection and leave callers to serialise around it: a connection
// carries one conversation at a time, and a Rows outlives the call that made it,
// so the alternative would be a lock held across someone else's iteration.
type PreparedStmt interface {
	// Query runs the statement and hands back every row. The caller owns the Rows
	// and must Close it, exactly as with Executor.QueryRowsRaw.
	Query(ctx context.Context, args ...any) (Rows, error)

	// Exec runs the statement and reports how many rows it changed, with the same
	// meaning — and the same silence about statements that change none — as
	// Executor.Exec.
	Exec(ctx context.Context, args ...any) (int64, error)

	// Close releases whatever the statement is holding, which may include a
	// connection. The statement is unusable afterwards.
	Close() error
}
