package kvdbs

import (
	"context"
	"time"

	"github.com/x64c/gwf/gw/clock"
)

// DB is one key-value database reached through a Client.
//
// TIME. Questions here are asked in the caller's terms — a lifetime is a
// time.Duration, and Exists reports whether the key is there now. An
// implementation answers them on its own clock, which advances in discrete
// ticks and holds nothing between them. Clock reports it.
//
// A lifetime is placed by reading both of its ends onto that clock, and the key
// then holds its final mark until the clock leaves it. Given a precision P,
// that is what a caller may rely on:
//
//   - The stored lifetime COVERS the one asked for. It can run over by less
//     than P; it does not fall short.
//   - A duration read back is accurate to within P.
//
// Both hold for any lifetime the clock can hold, which is P and up. Shorter
// than P may span no mark at all, depending where it falls, and a span of no
// marks is no time.
//
// Zero therefore means one thing everywhere it appears, whatever the verb: a
// lifetime already over, so the key goes. Nothing is stored for no time, and
// nothing outlives a lifetime of none. Storing without a lifetime is a separate
// operation with its own name (SetValuePersistent), as is dropping one (Persist), so
// neither has to be smuggled in as a duration that means its opposite.
//
// Lifetimes are not negative; an implementation rejects a negative duration
// rather than inventing a meaning for it.
//
// How an implementation gets there — where it anchors, which of its backend's
// commands it reaches for — is its own business. Meeting this at the P it
// publishes is not, so that any caller can predict its behavior from P alone,
// and check it.
//
// VALUES. What a store holds is a string, which is why GetValue and the hash readers
// return one. Writes take any as a convenience: an implementation accepts
// whichever types it can encode, and how it encodes them is its own business —
// this interface neither specifies that nor can see it.
//
// One guarantee, and it is the identity one: a string written is the same
// string read back, byte for byte. Nothing is promised about the stored form of
// any other type, nor about whether a given implementation accepts it at all.
//
// So: when the stored representation matters — a layout something else will
// parse, a value another implementation may later serve — produce it yourself
// and write a string. Passing a non-string is for values whose exact bytes you
// have no stake in. A time is the clearest case to convert yourself: RFC 3339,
// epoch seconds and epoch nanoseconds all encode an instant, and a stored one
// is meaningless unless every later reader agrees which was used.
type DB interface {
	// Clock is the clock this implementation answers time on. Its precision is
	// the tick the guarantees above are stated in, and it renders instants the
	// same way the implementation does, so a caller can place a deadline itself
	// rather than re-deriving the arithmetic. Constant for the life of the DB.
	Clock() clock.Clock

	// Ping checks the database answers. It reports the state at the moment it
	// returns and nothing beyond it: a connection can die between a successful
	// Ping and the next operation, so this is a health signal, not a guarantee
	// to act on.
	Ping(ctx context.Context) error

	//---- Key Ops ----

	Exists(ctx context.Context, key string) (bool, error)
	TTL(ctx context.Context, key string) (time.Duration, TTLState, error)
	Delete(ctx context.Context, keys ...string) (int64, error)
	// Expire gives a key a lifetime, ending it that far from now. Here the
	// duration IS the subject, so a lifetime of zero is one already over and
	// the key is removed — as is one shorter than the clock's precision, which
	// spans no mark and is therefore no time at all.
	Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) // found & updated, err
	// Persist removes a key's lifetime, leaving it stored with no expiry.
	// "Never expires" is an operation rather than a lifetime, so it has no
	// spelling as a duration and cannot be passed to Expire by mistake.
	//
	// Reports whether a lifetime was removed. False covers both a missing key
	// and one that already had none — TTL tells those apart.
	Persist(ctx context.Context, key string) (bool, error)
	// Type returns the string representation of the value type stored at the given key.
	Type(ctx context.Context, key string) (string, error)

	// ScanKeys iterates over keys in the database in batches.
	// Returns keys []string, nextCursor any, err error
	// It attempts to return up to scanBatchSize keys starting from the given cursor.
	// The exact number of keys returned may vary depending on the backend's scanning behavior.
	// The cursor type and meaning are backend-specific and opaque to callers.
	// When nextCursor is nil, the scan is complete.
	// Backends that do not support key iteration (e.g. Memcached) should return ErrNotSupported.
	ScanKeys(ctx context.Context, cursor any, scanBatchSize int) ([]string, any, error)

	//---- Single-value Ops ----

	// SetValue stores a value under key for the given lifetime. A lifetime of zero is
	// one already over, so the key is removed and the value never exists — as
	// with any lifetime too short to span a mark. Use SetValuePersistent to store
	// without a lifetime; there is no spelling of "no lifetime" as a duration.
	SetValue(ctx context.Context, key string, value any, expiration time.Duration) error
	// SetValuePersistent stores a value under key with no lifetime, removing any the
	// key already had.
	SetValuePersistent(ctx context.Context, key string, value any) error
	GetValue(ctx context.Context, key string) (string, bool, error) // val, found, err

	// Conditional writes — the check and the write are ONE atomic act in the
	// store, so among any number of concurrent callers exactly one wins.
	// The bool is the store's verdict: true = the key was absent and is now
	// set; false = the key already exists, untouched. When err is non-nil the
	// operation itself failed and the bool carries no information. Losing the
	// race is a normal outcome, not an error.
	//
	// The lifetime must span at least one mark: a zero or shorter lifetime has
	// no honorable atomic outcome for a conditional write (the key cannot be
	// written, and an existing key must not be touched), so it is an error —
	// unlike SetValue, whose zero collapses to removal.

	// SetValueIfNotExists stores the value under key ONLY if the key does not
	// exist, with the given lifetime.
	SetValueIfNotExists(ctx context.Context, key string, value any, expiration time.Duration) (bool, error)
	// SetValuePersistentIfNotExists stores the value under key ONLY if the key
	// does not exist, with no lifetime.
	SetValuePersistentIfNotExists(ctx context.Context, key string, value any) (bool, error)
	// DeleteValueIfEquals deletes key ONLY if its stored value equals expected,
	// compared in the stored string form — the one GetValue returns. The
	// comparison and the delete are one atomic act in the store. true = matched
	// and deleted; false = the key is absent or holds a different value, and
	// nothing was touched — to a caller giving back what it held under key,
	// those two are one answer: not yours any more.
	DeleteValueIfEquals(ctx context.Context, key string, expected any) (bool, error)
	// SetValueIfEquals stores value under key ONLY if the stored value equals
	// expected, compared in the stored string form; the key's lifetime, if
	// any, is kept. The comparison and the write are one atomic act in the
	// store. true = matched and written; false = the key is absent or holds
	// a different value, and nothing was touched.
	SetValueIfEquals(ctx context.Context, key string, expected any, value any) (bool, error)

	// Integer interpretation — values are stored as strings; the two methods
	// below interpret a value as a base-10 signed 64-bit integer, and error on
	// a value that cannot be read that way. They are the ONLY integer view:
	// a counter read through GetValue is its string form.

	// IncrementValue atomically adds delta to the integer under key and
	// returns the new total (an absent key counts from zero). It also ensures
	// the key carries a lifetime: the given lifetime is applied whenever the
	// key has none — including the moment this increment creates it — and an
	// existing lifetime is never extended or shortened. The lifetime must span
	// at least one mark; use IncrementValuePersistent for a counter with no
	// lifetime.
	IncrementValue(ctx context.Context, key string, delta int64, lifetime time.Duration) (int64, error)
	// IncrementValuePersistent is IncrementValue for a counter with no
	// lifetime: an absent key is created persistent, and an existing lifetime
	// is left untouched.
	IncrementValuePersistent(ctx context.Context, key string, delta int64) (int64, error)
	// GetValueAsInt64 reads the value under key through the same integer
	// interpretation IncrementValue writes with. found reports whether the key
	// exists; a value that does not parse as such an integer is an error.
	GetValueAsInt64(ctx context.Context, key string) (int64, bool, error)

	//---- List Ops ----

	ListPush(ctx context.Context, key string, value string) error
	ListPop(ctx context.Context, key string) (string, bool, error) // val, found, err
	ListLen(ctx context.Context, key string) (int64, error)
	ListRange(ctx context.Context, key string, start int64, stop int64) ([]string, error) // 0-basis, stop inclusive
	ListRemove(ctx context.Context, key string, cnt int64, value any) (int64, error)      // cnt = removed dups. 0 = all
	ListTrim(ctx context.Context, key string, start int64, stop int64) error              // 0-basis, stop inclusive
	// ListPushTrimOverCap appends value to the list at key, gives the key the
	// lifetime keyTTL, and, if the list is now longer than capMax, removes the
	// oldest entries over the cap and returns them, oldest first. The push, the
	// lifetime, and the trim are one atomic act in the store, so among
	// concurrent pushers the list never exceeds the cap, no entry is trimmed
	// twice, and the key is never without a lifetime. capMax must be > 0; the
	// lifetime must span at least one mark.
	ListPushTrimOverCap(ctx context.Context, key string, value string, capMax int64, keyTTL time.Duration) ([]string, error)

	//---- Hash Ops ----
	//
	// The writes come in two families, differing only in whether the key may be
	// created:
	//
	//	HashSet…          upsert: writes the fields, creating the key when absent.
	//	HashSet…IfExists  update-only: writes the fields when the key already
	//	              exists, and never creates it.
	//
	// Pick by whether creation is correct for the caller. Where a key's
	// existence is itself meaningful — absence means gone, or the key's
	// lifetime is owned elsewhere — the upsert family recreates it, and only
	// the IfExists family states what such a caller means.

	// HashSetField sets one field, creating the key if absent. A key created here
	// has no TTL; an existing key keeps the TTL it already had.
	HashSetField(ctx context.Context, key string, field string, value any) error
	// HashSetFieldWithKeyTTL atomically sets one field and assigns the key's TTL,
	// creating the key if absent. The TTL is assigned unconditionally: an
	// existing key's remaining lifetime is replaced by ttl.
	HashSetFieldWithKeyTTL(ctx context.Context, key string, field string, value any, ttl time.Duration) error
	// HashSetFields sets multiple fields, creating the key if absent. A key created
	// here has no TTL; an existing key keeps the TTL it already had.
	HashSetFields(ctx context.Context, key string, fields map[string]any) error
	// HashSetFieldsWithKeyTTL atomically sets multiple fields and assigns the key's
	// TTL, creating the key if absent. The TTL is assigned unconditionally: an
	// existing key's remaining lifetime is replaced by ttl.
	HashSetFieldsWithKeyTTL(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error

	// HashSetFieldIfExists sets one field only if the key already exists, and never
	// creates it. Reports whether the key existed, which is whether the write
	// happened. The key's TTL is left unchanged.
	//
	// The existence test and the write MUST be one indivisible operation. A
	// backend that performs them separately offers nothing over the caller
	// running Exists then HashSetField, which is the race this exists to remove; if
	// it cannot be done indivisibly, return ErrNotSupported rather than
	// emulating it.
	HashSetFieldIfExists(ctx context.Context, key string, field string, value any) (bool, error) // existed (and written), err
	// HashSetFieldWithKeyTTLIfExists atomically sets one field and assigns the key's
	// TTL, only if the key already exists, and never creates it. Reports
	// whether the key existed. Indivisible, as HashSetFieldIfExists requires.
	HashSetFieldWithKeyTTLIfExists(ctx context.Context, key string, field string, value any, ttl time.Duration) (bool, error)
	// HashSetFieldsIfExists sets multiple fields only if the key already exists,
	// and never creates it. Reports whether the key existed. The key's TTL is
	// left unchanged. Indivisible, as HashSetFieldIfExists requires.
	HashSetFieldsIfExists(ctx context.Context, key string, fields map[string]any) (bool, error)
	// HashSetFieldsWithKeyTTLIfExists atomically sets multiple fields and assigns the
	// key's TTL, only if the key already exists, and never creates it. Reports
	// whether the key existed. Indivisible, as HashSetFieldIfExists requires.
	HashSetFieldsWithKeyTTLIfExists(ctx context.Context, key string, fields map[string]any, ttl time.Duration) (bool, error)
	// HashSetFieldsWithKeyTTLIfFieldEquals atomically sets multiple fields and
	// assigns the key's TTL, only if the key exists AND its field equals
	// expected, compared in the stored string form; it never creates the key.
	// Reports whether the field matched, which is whether the write happened:
	// an absent key and a different value both answer false, untouched. The
	// hash form of SetValueIfEquals — among concurrent callers presenting the
	// same expected value, exactly one wins. Indivisible, as
	// HashSetFieldIfExists requires.
	HashSetFieldsWithKeyTTLIfFieldEquals(ctx context.Context, key string, field string, expected any, fields map[string]any, ttl time.Duration) (bool, error)

	HashGetField(ctx context.Context, key string, field string) (string, bool, error) // val, found, err
	// HashGetFields returns values of found fields. By comparing lengths, you can check if all fields are found.
	// Fields that are absent are omitted from the map, so a missing key and a key holding none of the
	// requested fields both return an empty map — use Exists to tell those apart.
	HashGetFields(ctx context.Context, key string, fields ...string) (map[string]string, error)
	// HashRemoveFields removes the specified fields in a hash key. Returns the number of fields actually removed.
	HashRemoveFields(ctx context.Context, key string, fields ...string) (int64, error)
	// HashGetAll returns every field of the hash. A missing key and an empty hash are
	// indistinguishable here — both return an empty map.
	HashGetAll(ctx context.Context, key string) (map[string]string, error)
}
