package authn

import "context"

type ctxKeyVerifiedIdentity struct{}

// WithVerifiedIdentity attaches an established identity to ctx. Middleware
// that authenticates a request places it here; handlers read it back with
// VerifiedIdentityFromContext.
func WithVerifiedIdentity(ctx context.Context, id VerifiedIdentity) context.Context {
	return context.WithValue(ctx, ctxKeyVerifiedIdentity{}, id)
}

// VerifiedIdentityFromContext returns the identity placed by
// WithVerifiedIdentity, if any.
func VerifiedIdentityFromContext(ctx context.Context) (VerifiedIdentity, bool) {
	id, ok := ctx.Value(ctxKeyVerifiedIdentity{}).(VerifiedIdentity)
	return id, ok
}
