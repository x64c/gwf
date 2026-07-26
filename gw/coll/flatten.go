package coll

import "github.com/x64c/gwf/gw/model"

// Flatten collects sub-collections from each item into a single flat collection.
// Duplicates (by ID) are skipped. O(total children) — each child visited once despite the nested loops.
func Flatten[
	SP model.Identifiable[SID],
	SID comparable,
	TP model.Identifiable[TID],
	TID comparable,
](
	src *Collection[SP, SID],
	extract func(SP) *Collection[TP, TID],
) *Collection[TP, TID] {
	result := NewEmptyOrderedCollection[TP, TID]()
	src.ForEach(func(s SP) {
		if sub := extract(s); sub != nil {
			sub.ForEach(func(t TP) {
				result.AddIfNew(t)
			})
		}
	})
	return result
}
