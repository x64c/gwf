package redis

import (
	"context"
	"errors"
	"fmt"
	"time"

	lowimpl "github.com/redis/go-redis/v9"
	"github.com/x64c/gwf/gw/kvdbs"
)

// DB implements kvdbs.DB for Redis.
type DB struct {
	internal *lowimpl.Client
}

// Ensure DB implements kvdbs.DB interface
var _ kvdbs.DB = (*DB)(nil)

//--- Key Ops ----

func (d *DB) Exists(ctx context.Context, key string) (bool, error) {
	n, err := d.internal.Exists(ctx, key).Result()
	return n > 0, err
}

func (d *DB) TTL(ctx context.Context, key string) (time.Duration, kvdbs.TTLState, error) {
	cmd := d.internal.TTL(ctx, key) // *redis.DurationCmd
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

func (d *DB) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	// Redis EXPIRE returns true if key existed and TTL was set, false if key does not exist
	return d.internal.Expire(ctx, key, expiration).Result()
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

func (d *DB) Get(ctx context.Context, key string) (string, bool, error) {
	val, err := d.internal.Get(ctx, key).Result()
	if errors.Is(err, lowimpl.Nil) {
		return "", false, nil // redis.Nil -> ok: false, err: nil
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

func (d *DB) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	return d.internal.Set(ctx, key, value, expiration).Err()
}

//---- List Ops ----

func (d *DB) Push(ctx context.Context, key, value string) error {
	// Append to the tail (right) of the list
	return d.internal.RPush(ctx, key, value).Err()
}

func (d *DB) Pop(ctx context.Context, key string) (string, bool, error) { // val, found, err
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

func (d *DB) Len(ctx context.Context, key string) (int64, error) {
	return d.internal.LLen(ctx, key).Result()
}

func (d *DB) Range(ctx context.Context, key string, start, stop int64) ([]string, error) {
	return d.internal.LRange(ctx, key, start, stop).Result()
}

func (d *DB) Remove(ctx context.Context, key string, cnt int64, value any) (int64, error) {
	return d.internal.LRem(ctx, key, cnt, value).Result()
}

func (d *DB) Trim(ctx context.Context, key string, start, stop int64) error {
	return d.internal.LTrim(ctx, key, start, stop).Err()
}

//---- Hash Ops ----

func (d *DB) SetField(ctx context.Context, key string, field string, value any) error {
	return d.internal.HSet(ctx, key, field, value).Err()
}

func (d *DB) SetFieldWithTTL(ctx context.Context, key string, field string, value any, ttl time.Duration) error {
	pipe := d.internal.TxPipeline()
	pipe.HSet(ctx, key, field, value)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (d *DB) SetFields(ctx context.Context, key string, fields map[string]any) error {
	return d.internal.HSet(ctx, key, fields).Err()
}

func (d *DB) SetFieldsWithTTL(ctx context.Context, key string, fields map[string]any, ttl time.Duration) error {
	pipe := d.internal.TxPipeline()
	pipe.HSet(ctx, key, fields)
	pipe.Expire(ctx, key, ttl)
	_, err := pipe.Exec(ctx)
	return err
}

func (d *DB) GetField(ctx context.Context, key string, field string) (string, bool, error) { // val, found, err
	val, err := d.internal.HGet(ctx, key, field).Result()
	if errors.Is(err, lowimpl.Nil) {
		return "", false, nil // key or field missing
	}
	if err != nil {
		return "", false, err
	}
	return val, true, nil
}

// GetFields returns a map {field:value} from a hash data, which contains only found fields
// so, if len(rtnMap) < len(fields), some fields are missing
// [NOTE] returns an empty map even if key is not found. not error
func (d *DB) GetFields(ctx context.Context, key string, fields ...string) (map[string]string, error) {
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

func (d *DB) RemoveFields(ctx context.Context, key string, fields ...string) (int64, error) {
	return d.internal.HDel(ctx, key, fields...).Result()
}

// GetAllFields returns a map {field:value} from a hash data with all fields in it
// [NOTE] returns an empty map even if key is not found. not error
func (d *DB) GetAllFields(ctx context.Context, key string) (map[string]string, error) {
	return d.internal.HGetAll(ctx, key).Result()
}
