package passwd

import "context"

// StoreRehashFunc is the app's replacement of a subject's stored encoding after a rehash — only
// while the stored one is still oldEncoding, so a password changed meanwhile, or another login's
// rehash, wins (in SQL: `… SET pw = $new WHERE subject = $s AND pw = $old`).
//
//   - Returns: nil — the encoding was replaced, or was no longer oldEncoding; an error only when
//     the store could not answer.
type StoreRehashFunc func(ctx context.Context, subject, oldEncoding, newEncoding string) error
