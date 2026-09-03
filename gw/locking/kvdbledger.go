package locking

import (
	"context"
	"strings"

	"github.com/x64c/gwf/gw/kvdbs"
)

// KVDBLedger is an external ledger held in a KVDB: one string row per
// standing grant at prefix + name, holding the proof. Every instance built
// over the same KVDB and prefix keeps the same external ledger. Grant rows
// carry no lifetime: a grant stands until the manager holding it releases
// it, or until cleanup removes it.
//
// Only ManagerKVDBLedger takes and releases here, and this type exports no
// way to do either. What it exports is inspection and administration.
type KVDBLedger struct {
	kvdb   kvdbs.DB
	prefix string // the ledger's own region of the keyspace: every row is prefix + name
}

// cleanupBatch bounds one scan and one delete during cleanup.
const cleanupBatch = 500

func (l *KVDBLedger) take(ctx context.Context, name, proof string) (bool, error) {
	return l.kvdb.SetValuePersistentIfNotExists(ctx, l.prefix+name, proof)
}

func (l *KVDBLedger) retire(ctx context.Context, name, proof string) (bool, error) {
	return l.kvdb.DeleteValueIfEquals(ctx, l.prefix+name, proof)
}

// Standing reports the proof standing on name, if any, as the external
// ledger has it now.
func (l *KVDBLedger) Standing(ctx context.Context, name string) (proof string, ok bool, err error) {
	return l.kvdb.GetValue(ctx, l.prefix+name)
}

// CleanupNames removes the given names from the external ledger regardless
// of who holds them, and reports how many stood. It is for an instance that
// died holding names and left them standing, never the regular release: an
// instance still holding one of them keeps it in its internal ledger, fails
// its own release here, and logs that. Run it when nothing is meant to hold.
func (l *KVDBLedger) CleanupNames(ctx context.Context, names []string) (int, error) {
	keys := make([]string, len(names))
	for i, name := range names {
		keys[i] = l.prefix + name
	}
	return l.deleteKeys(ctx, keys)
}

// Cleanup removes every row in the external ledger's region the same way
// CleanupNames does, for a fresh start. The KVDB scans by batch, not by
// prefix, so this walks the whole database: a cold path.
func (l *KVDBLedger) Cleanup(ctx context.Context) (int, error) {
	removed := 0
	var cursor any
	for {
		keys, next, err := l.kvdb.ScanKeys(ctx, cursor, cleanupBatch)
		if err != nil {
			return removed, err
		}
		ours := keys[:0]
		for _, key := range keys {
			if strings.HasPrefix(key, l.prefix) {
				ours = append(ours, key)
			}
		}
		n, err := l.deleteKeys(ctx, ours)
		removed += n
		if err != nil {
			return removed, err
		}
		if next == nil {
			return removed, nil
		}
		cursor = next
	}
}

// deleteKeys deletes keys in batches and reports how many existed.
func (l *KVDBLedger) deleteKeys(ctx context.Context, keys []string) (int, error) {
	removed := 0
	for len(keys) > 0 {
		batch := keys
		if len(batch) > cleanupBatch {
			batch = keys[:cleanupBatch]
		}
		n, err := l.kvdb.Delete(ctx, batch...)
		removed += int(n)
		if err != nil {
			return removed, err
		}
		keys = keys[len(batch):]
	}
	return removed, nil
}
