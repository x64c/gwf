package locking

import (
	"context"
	"errors"
	"log"
	"strconv"
	"sync/atomic"

	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/kvdbs"
	"github.com/x64c/gwf/gw/security"
)

// ManagerKVDBLedger is the Manager for an app that runs as several
// instances. It keeps its internal ledger as its own slice of an external
// ledger held in a KVDB, so that the instances' internal ledgers never
// contradict each other: every ask goes to the internal ledger first, and a
// name not held here is asked of the external ledger under a proof only this
// instance knows.
type ManagerKVDBLedger struct {
	name     string
	ledger   internalLedger[string] // lock name → the proof this instance's grant stands under
	external *KVDBLedger
	id       string // this instance's boot id: proofs are id:seq, unique across a fleet
	seq      atomic.Uint64
}

var _ Manager = (*ManagerKVDBLedger)(nil)

// NewManagerKVDBLedger builds a ManagerKVDBLedger named name over kvdb.
// prefix is the external ledger's own region of the keyspace, so a grant row
// and a data row never meet, and every instance building over the same kvdb
// and prefix keeps the same external ledger. None of the three may be empty.
func NewManagerKVDBLedger(name string, kvdb kvdbs.DB, prefix string) (*ManagerKVDBLedger, error) {
	if name == "" {
		return nil, errors.New("locking.NewManagerKVDBLedger: name required")
	}
	if kvdb == nil {
		return nil, errors.New("locking.NewManagerKVDBLedger: kvdb required")
	}
	if prefix == "" {
		return nil, errors.New("locking.NewManagerKVDBLedger: prefix required")
	}
	m := &ManagerKVDBLedger{
		name:     name,
		external: &KVDBLedger{kvdb: kvdb, prefix: prefix},
		id:       security.GenerateHex(8),
	}
	m.ledger.held = make(map[string]string)
	return m, nil
}

func (m *ManagerKVDBLedger) Name() string { return m.name }

// ExternalLedger is the external ledger this manager keeps in step with, for
// inspection and administration on a cold path. Taking and releasing stay
// the manager's.
func (m *ManagerKVDBLedger) ExternalLedger() *KVDBLedger { return m.external }

func (m *ManagerKVDBLedger) newProof() string {
	return m.id + ":" + strconv.FormatUint(m.seq.Add(1), 10)
}

func (m *ManagerKVDBLedger) AcquireDoRelease(ctx context.Context, name string, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	proof, err := m.acquire(ctx, name)
	if err != nil {
		return err
	}
	defer m.giveBack(name, proof)
	return runUnder(ctx, fn)
}

func (m *ManagerKVDBLedger) AcquireDoReleaseAll(ctx context.Context, names []string, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	proofs := make([]string, 0, len(names))
	giveBack := func() {
		for i := len(proofs) - 1; i >= 0; i-- {
			m.giveBack(names[i], proofs[i])
		}
	}
	for _, name := range names {
		proof, err := m.acquire(ctx, name)
		if err != nil {
			giveBack()
			return err
		}
		proofs = append(proofs, proof)
	}
	defer giveBack()
	return runUnder(ctx, fn)
}

func (m *ManagerKVDBLedger) Names() []string { return m.ledger.names() }

// acquire takes name for this instance: reserved in the internal ledger
// first, so a second local ask meanwhile is refused without a second trip,
// then asked of the external ledger under a fresh proof. A refusal by either
// leaves nothing behind.
func (m *ManagerKVDBLedger) acquire(ctx context.Context, name string) (string, error) {
	proof := m.newProof()
	if !m.ledger.reserve(name, proof) {
		return "", errs.ActionLocked.WithDetail(name)
	}
	won, err := m.external.take(ctx, name, proof)
	if err != nil {
		m.ledger.release(name)
		return "", err
	}
	if !won {
		m.ledger.release(name)
		return "", errs.ActionLocked.WithDetail(name)
	}
	return proof, nil
}

// giveBack releases name: at the external ledger first, presenting the
// proof, then in the internal ledger, so that a local ask arriving in
// between is refused by the internal ledger and not by this instance's own
// dying grant. The function is done, so the internal ledger is cleared
// whatever the external ledger answered; a row left standing is logged, and
// CleanupNames is the remedy. The call does not ride the caller's ctx, which
// may have ended: it is bounded by the KVDB client's own timeouts.
func (m *ManagerKVDBLedger) giveBack(name, proof string) {
	ok, err := m.external.retire(context.Background(), name, proof)
	switch {
	case err != nil:
		log.Printf("[WARN] locking(%s): releasing %q at the external ledger: %v (its row may stand; CleanupNames is the remedy)", m.name, name, err)
	case !ok:
		log.Printf("[WARN] locking(%s): releasing %q at the external ledger: the grant no longer stood under this instance's proof", m.name, name)
	}
	m.ledger.release(name)
}
