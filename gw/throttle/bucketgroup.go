package throttle

import (
	"sync"
	"time"
)

type BucketGroup struct {
	conf    *BucketConf
	buckets *sync.Map // string -> *Bucket
}

func (g *BucketGroup) GetBucket(id string) (*Bucket, bool) {
	bAny, ok := g.buckets.Load(id)
	if !ok {
		return nil, false
	}
	return bAny.(*Bucket), true
}

func (g *BucketGroup) SetBucket(id string, tokens int, now time.Time) {
	g.buckets.Store(id, &Bucket{
		tokens:      tokens,
		lastCheck:   now,
		parentGroup: g,
	})
}
