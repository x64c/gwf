package authn

import (
	"context"

	"github.com/x64c/gwf/gw/errs"
)

// UIDStrResolver maps a verified identity to the app's user, as the uid
// string a session stores, or refuses. It is the app's judgment entirely —
// which claim identifies the person, where users live, what "may log in"
// means — and the one hook every browser flow ends at. A refusal is answered
// by the caller as 401 with the returned error.
type UIDStrResolver func(ctx context.Context, id VerifiedIdentity) (uidStr string, e *errs.Error)
