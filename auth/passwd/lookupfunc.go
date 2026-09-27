package passwd

import "context"

// LookupFunc is the app's lookup of the password stored for a name: where encodings live and how
// a typed name finds its account (case, trimming) are the app's.
//
//   - Returns: the account's subject — the canonical name passwd.Verifier puts in the
//     authn.VerifiedIdentity — and its stored encoding, found true; found false for a name with
//     no password (unknown, or none set); an error only when the store could not answer.
type LookupFunc func(ctx context.Context, name string) (subject, encoding string, found bool, err error)
