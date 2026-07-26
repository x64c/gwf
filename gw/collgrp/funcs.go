package collgrp

import (
	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/model"
)

// GroupBy creates an Unordered CollectionGroup from a Collection
// You can give it a group order by Sort()
func GroupBy[
	MP model.Identifiable[ID],
	ID comparable,
	K comparable,
](
	srcColl *coll.Collection[MP, ID],
	keyFn func(MP) K,
) *CollectionGroup[MP, ID, K] {

	if srcColl == nil {
		return nil
	}

	g := NewEmptyCollectionGroup[MP, ID, K]()

	var subCollGen func() *coll.Collection[MP, ID]
	var getOrCreateSubCollection func(K) *coll.Collection[MP, ID]
	// respect the item-order of the source collection
	if srcColl.IsOrdered() {
		// source collection is ordered — sub-collections are ordered, and groups preserve first-appearance order
		subCollGen = coll.NewEmptyOrderedCollection
		getOrCreateSubCollection = func(k K) *coll.Collection[MP, ID] {
			if c, ok := g.FindCollection(k); ok {
				return c
			}
			c := subCollGen()
			g.SetCollection(k, c)
			g.order = append(g.order, k)
			return c
		}
	} else {
		subCollGen = coll.NewEmptyUnorderedCollection
		getOrCreateSubCollection = func(k K) *coll.Collection[MP, ID] {
			if c, ok := g.FindCollection(k); ok {
				return c
			}
			c := subCollGen()
			g.SetCollection(k, c)
			return c
		}
	}

	// Respect the source's natural iteration order (ordered or unordered)
	srcColl.ForEach(func(mp MP) {
		k := keyFn(mp)
		getOrCreateSubCollection(k).AddIfNew(mp)
	})

	return g
}
