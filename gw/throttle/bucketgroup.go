package throttle

import (
	"sync"
	"time"
)

// bucketMap is a typed facade over sync.Map. sync.Map's API is any-typed, so
// every read out of it needs an assertion; keeping them here leaves the rest of
// the package typed. sync.Map rather than map+RWMutex because a bucket map is
// read-mostly — a key is created once and then read for the life of its traffic
// — and RWMutex.RLock writes a shared counter on every request.
type bucketMap struct {
	m sync.Map // string -> *Bucket
}

func (bm *bucketMap) load(id string) (*Bucket, bool) {
	v, ok := bm.m.Load(id)
	if !ok {
		return nil, false
	}
	return v.(*Bucket), true
}

// loadOrStore returns the bucket already registered under id, or b if there was
// none. Exactly one of a set of concurrent callers has its bucket installed;
// every other one receives that same bucket. See loadOrCreateBucket.
func (bm *bucketMap) loadOrStore(id string, b *Bucket) *Bucket {
	v, _ := bm.m.LoadOrStore(id, b)
	return v.(*Bucket)
}

// rangeAll visits every bucket. f returns false to stop.
func (bm *bucketMap) rangeAll(f func(id string, b *Bucket) bool) {
	bm.m.Range(func(k, v any) bool {
		return f(k.(string), v.(*Bucket))
	})
}

func (bm *bucketMap) remove(id string) {
	bm.m.Delete(id)
}

type BucketGroup struct {
	conf    *BucketConf
	buckets *bucketMap
}

func (g *BucketGroup) GetBucket(id string) (*Bucket, bool) {
	return g.buckets.load(id)
}

// loadOrCreateBucket returns the bucket for id, creating a full one if absent.
//
// LoadOrStore, not load-then-store: concurrent first hits on one id must all
// come away holding the SAME bucket. Storing unconditionally lets every caller
// that missed install a bucket of its own and spend from a private full burst,
// so the group's Burst bounds nothing at the moment a key is born — and the
// last store wins, discarding the tokens the others consumed.
//
// The load first keeps the common path allocation-free: LoadOrStore must be
// handed a value whether or not it ends up storing it. It carries no decision —
// a stale miss just falls through to the atomic operation, which is
// authoritative.
func (g *BucketGroup) loadOrCreateBucket(id string, now time.Time) *Bucket {
	if b, ok := g.buckets.load(id); ok {
		return b
	}
	return g.buckets.loadOrStore(id, &Bucket{
		tokens:      g.conf.Burst,
		lastCheck:   now,
		parentGroup: g,
	})
}
