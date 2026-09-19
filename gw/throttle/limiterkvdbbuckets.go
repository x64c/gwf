package throttle

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/svc"
)

// LimiterKVDBBuckets is the Limiter whose token buckets live in a KVDB: one
// value per bucket, "<tokens>|<refilled at, unix nanos>", so every process
// running the app counts against the same buckets. What a group means and how
// fast it refills is the same as in memory — only the place the tokens are kept
// differs.
//
// Every write is conditional, and that is what makes the counting true:
//
//   - a bucket nobody holds yet is created with set-if-absent, so of many
//     callers meeting a fresh bucket together exactly ONE creates it and takes
//     its token; the rest read what it wrote;
//   - a bucket that exists is spent with compare-and-swap against the exact
//     value read, so of two instances spending the last token only one wins and
//     the loser reads again.
//
// A caller therefore pays a round trip per attempt, and an unreachable store
// yields an error, not a refusal: no verdict could be computed.
//
// REFUSALS ARE ANSWERED WITHOUT THE STORE. A bucket with no tokens cannot gain
// one before its next refill moment, which the read already reported, so this
// limiter keeps that moment per bucket and refuses locally until it passes. A
// caller hammering one bucket therefore reaches the KVDB at most once per refill
// period rather than once per request, which is what keeps an abusive client
// from turning a rate limit into a load generator. The memory is only ever a
// refusal: every admission comes from the store, and the moment kept is the
// store's own answer, so this never refuses what the store would have admitted —
// the shared bucket cannot refill sooner than its schedule allows.
//
// The bucket groups are boot wiring, as in memory: set them before the app
// serves, then only read them.
type LimiterKVDBBuckets struct {
	name       string                 // registered instance identity; see NewLimiterKVDBBucketsAs
	state      svc.AtomicState        // lifecycle state; what the framework's admission gate reads
	terminated chan error             // one-shot; fires when Terminate completes
	kvdb       kvdbs.DB               // where the buckets live
	prefix     string                 // this app's region of the keyspace: prefix + groupID + ":" + bucketID
	groups     map[string]*BucketConf // boot wiring: written before serving, read after

	mu       sync.RWMutex         // guards askAfter
	askAfter map[string]time.Time // bucket key → the moment the store is worth asking again
}

// casAttempts bounds one TryAdmit's races with other instances over the same
// bucket. Each lost attempt means another instance spent a token in between, so
// the next read sees fewer; a caller that loses this many in a row is told no
// verdict could be computed rather than being given a guess.
const casAttempts = 3

// denySweepAt is how many remembered refusals may stand before an add drops the
// passed ones. A refusal is remembered for at most one refill period, so the map
// follows how many distinct buckets were refused within one.
const denySweepAt = 1024

// NewLimiterKVDBBuckets builds the limiter over kvdb, keeping its buckets under
// prefix. The prefix is the app's own region of the keyspace: two apps sharing a
// KVDB must not share it, or they share their buckets.
func NewLimiterKVDBBuckets(kvdb kvdbs.DB, prefix string) *LimiterKVDBBuckets {
	return NewLimiterKVDBBucketsAs("ThrottleService", kvdb, prefix)
}

// NewLimiterKVDBBucketsAs is NewLimiterKVDBBuckets with the name given
// explicitly. A name identifies a registered INSTANCE, not a type: it is what
// logs, status output and dependency declarations all refer to, and registration
// rejects a duplicate. The string is taken raw — uniqueness and legibility are
// the caller's.
func NewLimiterKVDBBucketsAs(name string, kvdb kvdbs.DB, prefix string) *LimiterKVDBBuckets {
	l := &LimiterKVDBBuckets{
		name:       name,
		terminated: make(chan error, 1),
		kvdb:       kvdb,
		prefix:     prefix,
		groups:     make(map[string]*BucketConf),
		askAfter:   make(map[string]time.Time),
	}
	l.state.Store(svc.StateREADY)
	return l
}

// This limiter runs nothing. Its buckets live in the KVDB and expire there, and
// the refusals it remembers are dropped as new ones are added, so there is no
// goroutine to start, nothing to wind down, and no resource to release. The
// lifecycle below exists for one reason: the framework gates a service by its
// state, so Start is what makes this limiter usable and Stop is what withdraws
// it — no other work happens in either.

func (l *LimiterKVDBBuckets) Name() string { return l.name }

func (l *LimiterKVDBBuckets) State() svc.State { return l.state.Load() }

// Start : READY → RUNNING. parentCtx is unread — nothing here outlives a call.
// Lifecycle methods (Start/Stop/Terminate) are not safe to call concurrently.
func (l *LimiterKVDBBuckets) Start(_ context.Context) error {
	if l.state.Load() == svc.StateRUNNING {
		return nil // idempotent
	}
	if l.state.Load() != svc.StateREADY {
		return fmt.Errorf("cannot start: state is %v, must be READY", l.state.Load())
	}
	l.state.Store(svc.StateRUNNING)
	log.Printf("[INFO][%s] Running. (buckets in the KVDB)", l.Name())
	return nil
}

// Stop : RUNNING → READY. The buckets stay where they are; what stops is this
// limiter being usable.
func (l *LimiterKVDBBuckets) Stop(_ context.Context) error {
	if l.state.Load() == svc.StateREADY {
		return nil // idempotent
	}
	if l.state.Load() != svc.StateRUNNING {
		return fmt.Errorf("cannot stop: state is %v, must be RUNNING", l.state.Load())
	}
	l.state.Store(svc.StateREADY)
	log.Printf("[INFO][%s] Stopped.", l.Name())
	return nil
}

// Terminate : any → TERMINATING (irreversible). Fires Terminated.
func (l *LimiterKVDBBuckets) Terminate(_ context.Context) error {
	if l.state.Load() == svc.StateTERMINATING {
		return nil // idempotent
	}
	l.state.Store(svc.StateTERMINATING)
	log.Printf("[INFO][%s] Terminating.", l.Name())
	l.terminated <- nil // THE ONLY send site; unconditional, exactly once
	log.Printf("[INFO][%s] Terminated.", l.Name())
	return nil
}

func (l *LimiterKVDBBuckets) Terminated() <-chan error { return l.terminated }

// SetBucketGroup registers a bucket group by id, as the in-memory limiter does
// and under the same contract: groups are BOOT WIRING, written before anything
// serves and only read afterwards.
func (l *LimiterKVDBBuckets) SetBucketGroup(id string, conf *BucketConf) error {
	if conf == nil {
		return fmt.Errorf("throttle: bucket group %q: conf required", id)
	}
	if conf.Burst < 1 || conf.Increment < 1 || conf.IncrPeriod <= 0 {
		return fmt.Errorf("throttle: bucket group %q: burst, increment and period must be positive", id)
	}
	l.groups[id] = conf
	return nil
}

// HasGroup reports whether groupID names a registered group.
func (l *LimiterKVDBBuckets) HasGroup(groupID string) bool {
	_, ok := l.groups[groupID]
	return ok
}

// TryAdmit takes one token for bucketID within groupID and reports whether it got
// one — the verdict for one use.
//
//   - Returns: true when a token was spent in the store; false when the bucket is
//     empty, whether that was read now or already known from its last refusal; an
//     error when no verdict could be computed — the store did not answer, or the
//     races over this bucket were not settled within casAttempts. An unknown
//     groupID is blocked, as in memory.
//   - Uses: kvdbs.DB.HashGetFields, kvdbs.DB.HashSetFieldsWithKeyTTL,
//     kvdbs.DB.HashSetFieldsWithKeyTTLIfFieldEquals.
//
// Flow: a refusal still standing answers at once → read the bucket → refill it by
// the periods elapsed → empty: remember when it is worth asking again, refuse →
// otherwise write one token less, conditional on the refill moment read, and
// report the admission. A bucket the store does not hold yet is created full,
// minus the token this call takes.
func (l *LimiterKVDBBuckets) TryAdmit(ctx context.Context, groupID string, bucketID string, now time.Time) (bool, error) {
	conf, ok := l.groups[groupID]
	if !ok {
		return false, nil // unknown groupID -> always blocked
	}
	key := l.prefix + groupID + ":" + bucketID

	if l.refusalStands(key, now) {
		return false, nil
	}

	for range casAttempts {
		raw, found, err := l.kvdb.GetValue(ctx, key)
		if err != nil {
			return false, err // no verdict: the store did not answer
		}

		tokens, last, held := parseBucket(raw, found)
		if !held {
			// First sight of this bucket. Set-if-absent settles it in the store:
			// the one caller that lands the value takes the token it wrote, and
			// everyone else reads that value on the next turn of this loop. A
			// stored value this build cannot read is treated as absent, and this
			// same write replaces it.
			created, err := l.kvdb.SetValueIfNotExists(ctx, key, formatBucket(conf.Burst-1, now), idleTTL(conf))
			if err != nil {
				return false, err
			}
			if created {
				return true, nil
			}
			continue // someone created it first; read what they wrote
		}

		tokens, last = refilled(tokens, last, conf, now)
		if tokens <= 0 {
			l.rememberRefusal(key, last.Add(conf.IncrPeriod), now)
			return false, nil
		}

		swapped, err := l.kvdb.SetValueIfEquals(ctx, key, raw, formatBucket(tokens-1, last))
		if err != nil {
			return false, err
		}
		if swapped {
			// The swap keeps the key's lifetime, so the bucket of a caller who
			// never pauses would expire mid-use and come back full. Push the
			// expiry out to what an idle bucket is worth again.
			if _, err := l.kvdb.Expire(ctx, key, idleTTL(conf)); err != nil {
				return false, err
			}
			return true, nil
		}
		// Another instance moved the bucket first; read it again.
	}
	return false, fmt.Errorf("throttle: bucket %q in group %q: unsettled after %d attempts", bucketID, groupID, casAttempts)
}

// refusalStands reports whether this bucket's last refusal is still true — the
// moment the store itself named as its next refill has not arrived.
func (l *LimiterKVDBBuckets) refusalStands(key string, now time.Time) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	until, remembered := l.askAfter[key]
	return remembered && now.Before(until)
}

// rememberRefusal keeps the moment this bucket is worth asking about again, and
// drops the moments that have passed once the map has grown to denySweepAt.
func (l *LimiterKVDBBuckets) rememberRefusal(key string, until time.Time, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.askAfter) >= denySweepAt {
		for k, u := range l.askAfter {
			if !now.Before(u) {
				delete(l.askAfter, k)
			}
		}
	}
	l.askAfter[key] = until
}

// parseBucket reads a bucket's stored value, "<tokens>|<refilled at, unix
// nanos>". held is false when the store holds no such bucket, or holds one this
// build cannot read.
func parseBucket(raw string, found bool) (tokens int, last time.Time, held bool) {
	if !found {
		return 0, time.Time{}, false
	}
	rawTokens, rawLast, ok := strings.Cut(raw, "|")
	if !ok {
		return 0, time.Time{}, false
	}
	t, err := strconv.Atoi(rawTokens)
	if err != nil {
		return 0, time.Time{}, false
	}
	nanos, err := strconv.ParseInt(rawLast, 10, 64)
	if err != nil {
		return 0, time.Time{}, false
	}
	return t, time.Unix(0, nanos), true
}

// formatBucket is parseBucket's inverse: what a bucket looks like in the store.
func formatBucket(tokens int, last time.Time) string {
	return strconv.Itoa(tokens) + "|" + strconv.FormatInt(last.UnixNano(), 10)
}

// refilled is the in-memory bucket's refill, computed on values read from the
// store: whole periods only, capped at the group's burst, with the refill moment
// advanced by exactly the periods counted.
func refilled(tokens int, last time.Time, conf *BucketConf, now time.Time) (int, time.Time) {
	elapsed := now.Sub(last)
	if elapsed < conf.IncrPeriod {
		return tokens, last
	}
	times := int(elapsed / conf.IncrPeriod)
	tokens += times * conf.Increment
	if tokens > conf.Burst {
		tokens = conf.Burst
	}
	return tokens, last.Add(time.Duration(times) * conf.IncrPeriod)
}

// idleTTL is how long a bucket is kept without being touched: the time it takes
// to refill from empty to burst. A bucket idle for longer would be full, and a
// bucket the store has forgotten is created full — the same thing.
func idleTTL(conf *BucketConf) time.Duration {
	periods := (conf.Burst + conf.Increment - 1) / conf.Increment
	return time.Duration(periods) * conf.IncrPeriod
}

var (
	_ Limiter     = (*LimiterKVDBBuckets)(nil)
	_ svc.Service = (*LimiterKVDBBuckets)(nil)
)
