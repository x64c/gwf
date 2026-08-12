package sqldbs

import "context"

// Tx is the common transaction interface across supported databases.
// Only methods shared by major SQL databases are included.
// For driver-specific features, type-assert to the concrete Tx type.
//
// A Tx must be finished exactly once, by Commit or Rollback. Neither is
// optional: one left unfinished holds whatever its implementation reserved for
// it, and that is normally a connection.
//
// The ctx given to Commit and Rollback may be ignored. An implementation whose
// transaction takes its lifetime from the context it was begun with has no
// per-call context to honour, so cancelling one of these is not a way to abort.
type Tx interface {
	Executor

	// DB is the database this transaction runs on — a way to reach its Client, or
	// to hand a DB to something when only a Tx is in scope.
	DB() DB

	// Transaction Control

	// Commit makes every change since BeginTx permanent and finishes the
	// transaction.
	Commit(ctx context.Context) error

	// Rollback discards every change since BeginTx and finishes the transaction.
	//
	// On an already-finished transaction it returns an error rather than doing
	// nothing, and WHICH error is the implementation's own — this interface
	// publishes no sentinel for that state. Which makes the usual
	//
	//	defer tx.Rollback(ctx)
	//
	// safe but unexaminable: after a successful Commit the deferred call fails
	// harmlessly, and a caller cannot portably tell that apart from a rollback
	// that genuinely could not complete. Discard the error there, and check it
	// only where rolling back was the intended outcome.
	Rollback(ctx context.Context) error
}
