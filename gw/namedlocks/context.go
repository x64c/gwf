package namedlocks

import "context"

type ctxKey struct{}

func ContextWithAcquiredLocks(ctx context.Context, lockNames []string) context.Context {
	return context.WithValue(ctx, ctxKey{}, lockNames)
}

func AcquiredLockNamesFromContext(ctx context.Context) ([]string, bool) {
	ctxVal := ctx.Value(ctxKey{})
	val, ok := ctxVal.([]string)
	return val, ok // lockNames
}
