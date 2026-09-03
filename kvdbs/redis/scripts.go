package redis

import lowimpl "github.com/redis/go-redis/v9"

// Server-side scripts backing the verbs that must be one act.
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

// luaDeleteValueIfEquals deletes the key only when its stored value equals the
// given one — the fenced release: comparison and delete run as one script, so
// a key that expired and was rewritten by another owner is never deleted by
// the previous one. GET errors on a non-string key, and the script surfaces
// that as the command error it is.
//
//	KEYS[1] = key
//	ARGV[1] = expected value
const luaDeleteValueIfEquals = `
if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end
return 0`

// luaSetValueIfEquals writes the value only when the stored one equals the
// expected — a fenced replacement: the write lands only on the value the
// caller last saw, and KEEPTTL leaves the key's lifetime alone. GET errors
// on a non-string key, and the script surfaces that as the command error it
// is.
//
//	KEYS[1] = key
//	ARGV[1] = expected value
//	ARGV[2] = new value
const luaSetValueIfEquals = `
if redis.call('GET', KEYS[1]) == ARGV[1] then redis.call('SET', KEYS[1], ARGV[2], 'KEEPTTL') return 1 end
return 0`

// luaListPushTrimOverCap appends the value, assigns the key's lifetime, and
// trims the list back to the cap from its head, returning what it trimmed —
// push, PEXPIRE, and trim as one script, so concurrent pushers never trim
// each other's entries and the list is never without a lifetime. Entries are
// oldest at the head, as RPUSH leaves them.
//
//	KEYS[1] = key
//	ARGV[1] = value
//	ARGV[2] = cap
//	ARGV[3] = ttl in milliseconds
const luaListPushTrimOverCap = `
local n = redis.call('RPUSH', KEYS[1], ARGV[1])
redis.call('PEXPIRE', KEYS[1], ARGV[3])
local over = n - tonumber(ARGV[2])
if over <= 0 then return {} end
local evicted = redis.call('LRANGE', KEYS[1], 0, over - 1)
redis.call('LTRIM', KEYS[1], over, -1)
return evicted`

// luaSetFieldsWithTTLIfFieldEquals writes fields and assigns the key's TTL
// only when one field currently holds the expected value — the compare and
// the write as one script, so among concurrent callers presenting the same
// expected value exactly one lands. An absent key (HGET → false) never
// matches, so nothing is created.
//
//	KEYS[1] = key
//	ARGV[1] = field compared
//	ARGV[2] = expected value
//	ARGV[3] = ttl in milliseconds
//	ARGV[4:] = field, value, field, value, …
const luaSetFieldsWithTTLIfFieldEquals = `
local cur = redis.call('HGET', KEYS[1], ARGV[1])
if cur == false or cur ~= ARGV[2] then return 0 end
if tonumber(ARGV[3]) <= 0 then redis.call('DEL', KEYS[1]) return 1 end
for i = 4, #ARGV, 2 do redis.call('HSET', KEYS[1], ARGV[i], ARGV[i+1]) end
redis.call('PEXPIRE', KEYS[1], ARGV[3])
return 1`

var (
	scriptSetFieldsIfExists             = lowimpl.NewScript(luaSetFieldsIfExists)
	scriptSetFieldsWithTTLIfExists      = lowimpl.NewScript(luaSetFieldsWithTTLIfExists)
	scriptSetFieldsWithTTLIfFieldEquals = lowimpl.NewScript(luaSetFieldsWithTTLIfFieldEquals)
	scriptDeleteValueIfEquals           = lowimpl.NewScript(luaDeleteValueIfEquals)
	scriptSetValueIfEquals              = lowimpl.NewScript(luaSetValueIfEquals)
	scriptListPushTrimOverCap           = lowimpl.NewScript(luaListPushTrimOverCap)
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
