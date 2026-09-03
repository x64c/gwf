package locking

import "context"

type ctxKeyAcquiredLockNames struct{}

// ContextWithAcquiredLocks carries the names a request holds, so code
// downstream of whatever acquired them can read what it already holds rather
// than asking again.
func ContextWithAcquiredLocks(ctx context.Context, lockNames []string) context.Context {
	return context.WithValue(ctx, ctxKeyAcquiredLockNames{}, lockNames)
}

func AcquiredLockNamesFromContext(ctx context.Context) ([]string, bool) {
	ctxVal := ctx.Value(ctxKeyAcquiredLockNames{})
	val, ok := ctxVal.([]string)
	return val, ok // lockNames
}
