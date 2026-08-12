package redis

import lowimpl "github.com/redis/go-redis/v9"

// Server-side scripts backing the conditional hash writes.
//
// Redis has no key-level conditional flag on HSET — HSETNX tests a FIELD and
// creates the key regardless — so the existence test and the write are made
// indivisible by running them as one script: Redis executes a script to
// completion with no other command interleaved.
//
// Each returns 1 when the key existed and the fields were written, 0 when the
// key did not exist and nothing was created.
//
// Fields are written one HSET per pair rather than through unpack(ARGV), whose
// argument count is bounded by the Lua stack. The extra redis.call cost is
// per-field and inside a single atomic script.

// luaSetFieldsIfExists writes fields, leaving the key's TTL alone.
//
//	KEYS[1] = key
//	ARGV    = field, value, field, value, …
const luaSetFieldsIfExists = `
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
for i = 1, #ARGV, 2 do redis.call('HSET', KEYS[1], ARGV[i], ARGV[i+1]) end
return 1`

// luaSetFieldsWithTTLIfExists writes fields and assigns the key's TTL.
// PEXPIRE, not EXPIRE, so the caller's duration is honored as given.
//
//	KEYS[1] = key
//	ARGV[1] = ttl in milliseconds
//	ARGV[2:] = field, value, field, value, …
const luaSetFieldsWithTTLIfExists = `
if redis.call('EXISTS', KEYS[1]) == 0 then return 0 end
if tonumber(ARGV[1]) <= 0 then redis.call('DEL', KEYS[1]) return 1 end
for i = 2, #ARGV, 2 do redis.call('HSET', KEYS[1], ARGV[i], ARGV[i+1]) end
redis.call('PEXPIRE', KEYS[1], ARGV[1])
return 1`

var (
	scriptSetFieldsIfExists        = lowimpl.NewScript(luaSetFieldsIfExists)
	scriptSetFieldsWithTTLIfExists = lowimpl.NewScript(luaSetFieldsWithTTLIfExists)
)

// scriptFieldArgs flattens fields into the field,value,… form the scripts
// expect, following any leading arguments.
func scriptFieldArgs(head []any, fields map[string]any) []any {
	args := make([]any, 0, len(head)+len(fields)*2)
	args = append(args, head...)
	for f, v := range fields {
		args = append(args, f, v)
	}
	return args
}
