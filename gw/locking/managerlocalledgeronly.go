package locking

import (
	"context"
	"errors"

	"github.com/x64c/gwf/gw/errs"
)

// ManagerLocalLedgerOnly is the Manager for an app that runs as one
// instance. Its internal ledger is the whole truth: nothing outside the
// process can hold a name, so no proof and no external step exist.
type ManagerLocalLedgerOnly struct {
	name   string
	ledger internalLedger[struct{}]
}

var _ Manager = (*ManagerLocalLedgerOnly)(nil)

// NewManagerLocalLedgerOnly builds a ManagerLocalLedgerOnly named name, which
// may not be empty.
func NewManagerLocalLedgerOnly(name string) (*ManagerLocalLedgerOnly, error) {
	if name == "" {
		return nil, errors.New("locking.NewManagerLocalLedgerOnly: name required")
	}
	m := &ManagerLocalLedgerOnly{name: name}
	m.ledger.held = make(map[string]struct{})
	return m, nil
}

func (m *ManagerLocalLedgerOnly) Name() string { return m.name }

func (m *ManagerLocalLedgerOnly) AcquireDoRelease(ctx context.Context, name string, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !m.ledger.reserve(name, struct{}{}) {
		return errs.ActionLocked.WithDetail(name)
	}
	defer m.ledger.release(name)
	return runUnder(ctx, fn)
}

func (m *ManagerLocalLedgerOnly) AcquireDoReleaseAll(ctx context.Context, names []string, fn func(context.Context) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	taken := make([]string, 0, len(names))
	giveBack := func() {
		for i := len(taken) - 1; i >= 0; i-- {
			m.ledger.release(taken[i])
		}
	}
	for _, name := range names {
		if !m.ledger.reserve(name, struct{}{}) {
			giveBack()
			return errs.ActionLocked.WithDetail(name)
		}
		taken = append(taken, name)
	}
	defer giveBack()
	return runUnder(ctx, fn)
}

func (m *ManagerLocalLedgerOnly) Names() []string { return m.ledger.names() }
