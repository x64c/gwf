package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	lowimpl "github.com/redis/go-redis/v9"
	"github.com/x64c/gwf/gw/clock"
	"github.com/x64c/gwf/gw/kvdbs"
)

// TimePrecision is the smallest difference in time this driver can represent.
//
// Redis's millisecond commands (PEXPIRE, PTTL) are the finest it offers, and
// are the only ones used here. The second-granularity commands are deliberately
// avoided: EXPIRE truncates a duration DOWN to whole seconds, so Expire(1900ms)
// would store 1s and the key would die 900ms early.
const TimePrecision = time.Millisecond

// DB implements kvdbs.DB for Redis. Every lifetime it reads or writes is
// expressed on clock, which ticks at TimePrecision.
type DB struct {
	internal *lowimpl.Client
	clock    clock.Clock
}

// Ensure DB implements kvdbs.DB interface
var _ kvdbs.DB = (*DB)(nil)

//--- Key Ops ----

func (d *DB) Exists(ctx context.Context, key string) (bool, error) {
	n, err := d.internal.Exists(ctx, key).Result()
	return n > 0, err
}

func (d *DB) TTL(ctx context.Context, key string) (time.Duration, kvdbs.TTLState, error) {
	cmd := d.internal.PTTL(ctx, key) // *redis.DurationCmd
	if err := cmd.Err(); err != nil {
		return 0, 0, err
	}
	d2 := cmd.Val() // time.Duration
	if d2 == -1 {
		// -1: Persistent
		return 0, kvdbs.TTLPersistent, nil
	}
	if d2 < 0 {
		// -2: KeyNotFound
		// other neg values also fallback to KeyNotFound
		return 0, kvdbs.TTLKeyNotFound, nil
	}
	return d2, kvdbs.TTLExpiring, nil
}

func (d *DB) Delete(ctx context.Context, keys ...string) (int64, error) {
	return d.internal.Del(ctx, keys...).Result()
}

// Clock is the clock this driver answers time on.
func (d *DB) Clock() clock.Clock {
	return d.clock
}

// lifetime is what to hand Redis for a requested ttl: the ticks that stretch
// occupies, starting now.
//
// Redis anchors a relative expiry at its OWN reading of now, so giving it those
// ticks puts the deadline on the mark the caller's interval ends in, and the key
// holds that mark until the clock leaves it — covering all of [now, now+ttl).
// Handing over the raw duration instead would have Redis anchor at its reading
// of now and add its own truncation of ttl, which is a different quantity and
// can land a tick short.
func (d *DB) lifetime(ttl time.Duration) time.Duration {
	return d.clock.ReadDurationFrom(time.Now(), ttl)
}

// ErrNegativeLifetime is returned for a negative lifetime, which kvdbs.DB gives
// no meaning.
var ErrNegativeLifetime = errors.New("kvdbs/redis: lifetime must not be negative")

func (d *DB) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	if expiration < 0 {
		return false, fmt.Errorf("%w: %v", ErrNegativeLifetime, expiration)
	}
	// A span of nothing is a lifetime already over, and PEXPIRE with 0 removes
	// the key — exactly that. Otherwise PEXPIRE reports whether the key was
	// there to be given one.
	return d.internal.PExpire(ctx, key, d.lifetime(expiration)).Result()
}

// Persist removes a key's lifetime. Reports whether one was removed, which is
// false both for a missing key and for one that already had none.
func (d *DB) Persist(ctx context.Context, key string) (bool, error) {
	return d.internal.Persist(ctx, key).Result()
}

func (d *DB) Type(ctx context.Context, key string) (string, error) {
	return d.internal.Type(ctx, key).Result()
}

func (d *DB) ScanKeys(ctx context.Context, cursor any, scanBatchSize int) ([]string, any, error) {
	var cur uint64
	if cursor != nil {
		cur = cursor.(uint64)
	}
	keys, nextCursor, err := d.internal.Scan(ctx, cur, "*", int64(scanBatchSize)).Result()
	if err != nil {
		return nil, nil, err
	}
	// Redis returns nextCursor == 0 when the scan is complete.
	if nextCursor == 0 {
		return keys, nil, nil
	}
	return keys, nextCursor, nil
}

//---- Single-value Ops ----

func (d *DB) ValueGet(ctx context.Context, key string) (string, bool, error) {
	val, err := d.internal.Get(ctx, key).Result()
	if errors.Is(err, lowimpl.Nil) {
		return "", false, nil // redis.Nil -> ok: false, err: nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (d *DB) ValueSet(ctx context.Context, key string, value any, expiration time.Duration) error {
	if expiration < 0 {
		return fmt.Errorf("%w: %v", ErrNegativeLifetime, expiration)
	}
	if ttl := d.lifetime(expiration); ttl > 0 {
		return d.internal.Set(ctx, key, value, ttl).Err()
	}
	// The lifetime spans nothing, so the value would never exist. Removing the
	// key is that outcome in one command — and SET with PX 0 is an error anyway.
	return d.internal.Del(ctx, key).Err()
}

// ValueSetPersistent stores a value with no lifetime. A plain SET carries no expiry
// option and discards whatever the key already had.
func (d *DB) ValueSetPersistent(ctx context.Context, key string, value any) error {
	return d.internal.Set(ctx, key, value, 0).Err()
}

//---- List Ops ----

func (d *DB) ListPush(ctx context.Context, key, value string) error {
	// Append to the tail (right) of the list
	return d.internal.RPush(ctx, key, value).Err()
}

func (d *DB) ListPop(ctx context.Context, key string) (string, bool, error) { // val, found, err
	// Pop from the head (left) of the list (FIFO)
	val, err := d.internal.LPop(ctx, key).Result()
	if errors.Is(err, lowimpl.Nil) {
		return "", false, nil // redis.Nil -> ok: false, err: nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (d *DB) ListLen(ctx context.Context, key string) (int64, error) {
	return d.internal.LLen(ctx, key).Result()
}

func (d *DB) ListRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return d.internal.LRange(ctx, key, start, stop).Result()
}

func (d *DB) ListRemove(ctx context.Context, key string, cnt int64, value any) (int64, error) {
	return d.internal.LRem(ctx, key, cnt, value).Result()
}

func (d *DB) ListTrim(ctx context.Context, key string, start, stop int64) error {
	return d.internal.LTrim(ctx, key, start, stop).Err()
}

//---- Hash Ops ----

func (d *DB) HashSetField(ctx context.Context, key string, field string, value any) error {
	return d.internal.HSet(ctx, key, field, value).Err()
}

func (d *DB) HashSetFieldWithKeyTTL(ctx context.Context, key string, field string, value any, ttl time.Duration) error {
	return d.HashSetFieldsWithKeyTTL(ctx, key, map[string]any{field: value}, ttl)
}

func (d *DB) HashSetFields(ctx context.Context, key string, fields map[string]any) error {
	return d.internal.HSet(ctx, key, fields).Err()
}

func (d *DB) HashSetFieldsWithKeyTTL(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error {
	if ttl < 0 {
		return fmt.Errorf("%w: %v", ErrNegativeLifetime, ttl)
	}
	span := d.lifetime(ttl)
	if span <= 0 {
		// Spans nothing, so the fields would never be readable. Removing the key
		// is that outcome, and PEXPIRE 0 would remove it a step later anyway.
		return d.internal.Del(ctx, key).Err()
	}
	pipe := d.internal.TxPipeline()
	pipe.HSet(ctx, key, fields)
	pipe.PExpire(ctx, key, span)
	_, err := pipe.Exec(ctx)
	return err
}

func (d *DB) HashSetFieldIfExists(ctx context.Context, key string, field string, value any) (bool, error) {
	return d.HashSetFieldsIfExists(ctx, key, map[string]any{field: value})
}

func (d *DB) HashSetFieldWithKeyTTLIfExists(ctx context.Context, key string, field string, value any, ttl time.Duration) (bool, error) {
	return d.HashSetFieldsWithKeyTTLIfExists(ctx, key, map[string]any{field: value}, ttl)
}

func (d *DB) HashSetFieldsIfExists(ctx context.Context, key string, fields map[string]any) (bool, error) {
	existed, err := scriptSetFieldsIfExists.Run(ctx, d.internal, []string{key},
		scriptFieldArgs(nil, fields)...).Int64()
	if err != nil {
		return false, err
	}
	return existed == 1, nil
}

// HashSetFieldsWithKeyTTLIfExists writes the fields only if the key already exists.
// A lifetime spanning nothing removes the key, and still reports true — the key
// was there, and the write was applied by ending it.
func (d *DB) HashSetFieldsWithKeyTTLIfExists(ctx context.Context, key string, fields map[string]any, ttl time.Duration) (bool, error) {
	if ttl < 0 {
		return false, fmt.Errorf("%w: %v", ErrNegativeLifetime, ttl)
	}
	existed, err := scriptSetFieldsWithTTLIfExists.Run(ctx, d.internal, []string{key},
		scriptFieldArgs([]any{d.lifetime(ttl).Milliseconds()}, fields)...).Int64()
	if err != nil {
		return false, err
	}
	return existed == 1, nil
}

func (d *DB) HashGetField(ctx context.Context, key string, field string) (string, bool, error) { // val, found, err
	val, err := d.internal.HGet(ctx, key, field).Result()
	if errors.Is(err, lowimpl.Nil) {
		return "", false, nil // key or field missing
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// HashGetFields returns a map {field:value} from a hash, containing only found fields
// so, if len(rtnMap) < len(fields), some fields are missing
// [NOTE] returns an empty map even if key is not found. not error
func (d *DB) HashGetFields(ctx context.Context, key string, fields ...string) (map[string]string, error) {
	resultSlice, err := d.internal.HMGet(ctx, key, fields...).Result() // []any
	if err != nil {
		return nil, err
	}
	rtnMap := make(map[string]string, len(fields)) // capacity = max len = when all fields found
	for i, v := range resultSlice {
		if v != nil {
			rtnMap[fields[i]] = fmt.Sprint(v)
		}
		// if v is nil, field missing → omitted in the return map
	}
	return rtnMap, nil
}

func (d *DB) HashRemoveFields(ctx context.Context, key string, fields ...string) (int64, error) {
	return d.internal.HDel(ctx, key, fields...).Result()
}

// HashGetAll returns a map {field:value} from a hash with all its fields
// [NOTE] returns an empty map even if key is not found. not error
func (d *DB) HashGetAll(ctx context.Context, key string) (map[string]string, error) {
	return d.internal.HGetAll(ctx, key).Result()
}
