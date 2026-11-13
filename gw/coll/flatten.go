package coll

import (
	"fmt"

	"github.com/x64c/gwf/gw/model"
)

// Flatten merges the sub-collections hanging off src's items into one
// collection. src is a collection of parents whose items each carry a wired
// sub-collection — typically a HasMany relation field — and extract returns
// that field for an item.
//
// Order: the parents in src's order (the outer loop); within each parent, its
// sub-collection's items in that sub-collection's order. A child ID already in
// the result is skipped, so a child reached again through a later parent keeps
// its first place. src and every sub-collection must therefore be ordered: an
// unordered one is refused before anything is merged (Sort it first).
//
// A nil src or a nil sub-collection is refused too: an unloaded relation is
// nil, while a loaded one with no items is an empty collection.
// O(parents + children).
func (src *Collection[SP, SID]) Flatten[
	TP model.Identifiable[TID],
	TID comparable,
](
	extract func(SP) *Collection[TP, TID],
) (*Collection[TP, TID], error) {
	if src == nil {
		return nil, fmt.Errorf("coll: Flatten on a nil src — not loaded or not built")
	}
	if !src.IsOrdered() {
		return nil, fmt.Errorf("coll: Flatten needs an ordered src — Sort it first")
	}
	// phase 1 — gather and check: every input known before any merging
	subs := make([]*Collection[TP, TID], 0, src.Len())
	total := 0
	for s := range src.Iter() {
		sub := extract(s)
		if sub == nil {
			return nil, fmt.Errorf("coll: Flatten found a nil sub-collection on src item %v — not loaded or not built", s.GetID())
		}
		if !sub.IsOrdered() {
			return nil, fmt.Errorf("coll: Flatten needs ordered sub-collections — the one on src item %v is unordered; Sort it first", s.GetID())
		}
		subs = append(subs, sub)
		total += sub.Len()
	}
	// phase 2 — merge only, into a result sized once (repeats overcount: an upper bound)
	result := &Collection[TP, TID]{
		itemsMap: make(map[TID]TP, total),
		order:    make([]TID, 0, total),
	}
	for _, sub := range subs {
		sub.ForEach(func(t TP) {
			result.AddIfNew(t)
		})
	}
	return result, nil
}
