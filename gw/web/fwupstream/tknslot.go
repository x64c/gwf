package fwupstream

import (
	"sync"

	"github.com/x64c/gwf/gw/errs"
)

// TknSlot caches the result of one upstream-token KVDB read on SessionData.
// The Once guards a single load per (clientID, type) per request, so concurrent
// goroutines spawned by the handler share one fetch + cached result/error.
type TknSlot struct {
	Once sync.Once
	Val  string
	Err  *errs.Error
}

// NewDoneTknSlot returns a TknSlot pre-populated with val and its Once
// already in the "done" state — future Do calls return val directly.
func NewDoneTknSlot(val string) *TknSlot {
	s := &TknSlot{Val: val}
	s.Once.Do(func() {})
	return s
}
