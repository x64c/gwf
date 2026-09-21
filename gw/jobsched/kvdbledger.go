package jobsched

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/x64c/gwf/gw/kvdbs"
)

// markLifetime is how long a claim's mark is kept. An instance derives the
// minute it is working on from its own clock (now.Unix()/60), so it attempts a
// planned minute only while it is inside it and never computes that key again
// afterwards: the mark has to outlive one tick period plus whatever the
// instances' clocks disagree by, and nothing beyond that. Generous is cheap —
// too short lets a badly skewed instance find no mark and run the job again,
// while too long only keeps keys nobody reads.
const markLifetime = 5 * time.Minute

// KVDBLedger is the LedgerForJobsPerAppCrossProc kept in a KVDB: one key per
// planned minute, created only if absent, so of all the instances reaching the
// same planned minute exactly
// one is told it won. The store settles it in a single conditional write —
// there is nothing to lock and nothing to release.
//
// A mark is NOT a history. It exists only while the minute it names can still
// be attempted, then the store drops it: nothing sweeps, and the keyspace holds
// only the last few minutes of claimed jobs. What ran, when, and how it ended
// belongs to logging.
//
// The mark also outlives the run it authorized, on purpose. Releasing it when
// the work finished would offer the same planned minute to a late instance,
// which is exactly the second run it exists to prevent.
//
// The stored value is the moment the claim was taken, in unix seconds. Nothing
// reads it; it is there so a key is legible to an operator reading the store.
type KVDBLedger struct {
	kvdb   kvdbs.DB
	prefix string
}

// NewKVDBLedger builds the ledger over kvdb, keeping its marks under prefix.
// The prefix is the region: everything sharing it shares the ledger, so it
// carries the app's name, and nothing else of the app may write there.
func NewKVDBLedger(kvdb kvdbs.DB, prefix string) (*KVDBLedger, error) {
	if kvdb == nil {
		return nil, fmt.Errorf("jobsched.NewKVDBLedger: kvdb required")
	}
	if prefix == "" {
		return nil, fmt.Errorf("jobsched.NewKVDBLedger: prefix required")
	}
	return &KVDBLedger{kvdb: kvdb, prefix: prefix}, nil
}

var _ LedgerForJobsPerAppCrossProc = (*KVDBLedger)(nil)

// Claim creates the mark for jobID's plannedMinute and reports true; an
// instance finding the mark already there reports false and leaves the job to
// whoever holds it.
func (l *KVDBLedger) Claim(ctx context.Context, jobID string, plannedMinute int64) (bool, error) {
	key := l.prefix + jobID + ":" + strconv.FormatInt(plannedMinute, 10)
	won, err := l.kvdb.SetValueIfNotExists(ctx, key, strconv.FormatInt(time.Now().Unix(), 10), markLifetime)
	if err != nil {
		return false, fmt.Errorf("jobsched ledger: %w", err)
	}
	return won, nil
}
