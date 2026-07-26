package sqldbs

import "context"

// Tx is the common transaction interface across supported databases.
// Only methods shared by major SQL databases are included.
// For driver-specific features, type-assert to the concrete Tx type.
type Tx interface {
	Executor

	// DB - Back reference to the parent DB
	DB() DB

	// Transaction Control

	// Commit - Commit the transaction — all changes become permanent
	Commit(ctx context.Context) error
	// Rollback - Rollback the transaction — discard all changes since BeginTx
	Rollback(ctx context.Context) error
}
