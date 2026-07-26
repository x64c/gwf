package coll

import "github.com/x64c/gwf/gw/model"

// EnumerateToSlice iterates over every model in the collection and calls yield for each.
// Every model contributes exactly one value. No skipping.
// Conceptually equivalent to: [yield(m) for m in c].
func EnumerateToSlice[
	MP model.Identifiable[ID],
	ID comparable,
	V any,
](
	c *Collection[MP, ID],
	yield func(MP) V,
) []V {
	size := c.Len()
	// new slice with the fixed length
	sl := make([]V, size)
	// With the fixed length, we don't use ForEach to avoid sl = append(sl, v) for better performance
	if c.order != nil {
		i := 0
		for _, id := range c.order {
			if mp, ok := c.itemsMap[id]; ok {
				sl[i] = yield(mp)
				i++
			}
		}
		return sl[:i] // handles the skipped gracefully
	}
	i := 0
	for _, mp := range c.itemsMap {
		sl[i] = yield(mp)
		i++
	}
	return sl[:i]
}

// EnumerateToMap iterates over every model in the collection and calls yield for each.
// Every model contributes exactly one key–value pair. No skipping.
// Conceptually equivalent to: {k: v for m in c}.
func EnumerateToMap[
	MP model.Identifiable[ID],
	ID comparable,
	K comparable,
	V any,
](
	c *Collection[MP, ID],
	yield func(MP) (K, V),
) map[K]V {
	m := make(map[K]V, c.Len()) // new map
	c.ForEachUnorderly(func(mp MP) {
		k, v := yield(mp)
		m[k] = v
	})
	return m
}

// CollectToSlice iterates over the collection and calls yield for each model.
// If yield returns nil, the element is skipped (conditional yield).
// Returns a slice of yielded values.
// Equivalent to a list comprehension: [yield(m) for m in c if yield(m) != nil].
func CollectToSlice[
	MP model.Identifiable[ID],
	ID comparable,
	V any,
](
	c *Collection[MP, ID],
	yield func(MP) *V,
) []V {
	sl := make([]V, 0, c.Len()) // new slice
	c.ForEach(func(mp MP) {
		if vp := yield(mp); vp != nil {
			sl = append(sl, *vp)
		}
	})
	return sl
}

// CollectToMap iterates over the collection and calls yield for each model.
// If yield returns nil, the element is skipped (conditional yield).
// The yielded key–value pair determines each map entry.
// ToDo: Review Wrong Result due to Pointer Caching
func CollectToMap[
	MP model.Identifiable[ID],
	ID comparable,
	K comparable,
	V any,
](
	c *Collection[MP, ID],
	yield func(MP) (*K, *V),
) map[K]V {
	m := make(map[K]V, c.Len()) // new map
	c.ForEachUnorderly(func(mp MP) {
		if kp, vp := yield(mp); kp != nil && vp != nil {
			m[*kp] = *vp
		}
	})
	return m
}

func CollectUniqueToSlice[
	MP model.Identifiable[ID],
	ID comparable,
	V comparable,
](
	c *Collection[MP, ID],
	yield func(MP) V,
) []V {
	sl := make([]V, 0, c.Len())
	seen := make(map[V]struct{}, c.Len())
	if len(c.order) > 0 {
		// Ordered iteration: preserve first occurrence order
		for _, id := range c.order {
			item, ok := c.itemsMap[id]
			if !ok {
				continue
			}
			v := yield(item)
			if _, exists := seen[v]; !exists {
				seen[v] = struct{}{}
				sl = append(sl, v)
			}
		}
		return sl
	}
	// Unordered
	for _, item := range c.itemsMap {
		v := yield(item)
		if _, exists := seen[v]; !exists {
			seen[v] = struct{}{}
			sl = append(sl, v)
		}
	}
	return sl
}

func CollectUniqueToSliceWithSkip[
	MP model.Identifiable[ID],
	ID comparable,
	V comparable,
](
	c *Collection[MP, ID],
	yield func(MP) V,
	skip func(V) bool, // nil = no skip rule
) []V {
	sl := make([]V, 0, c.Len())
	seen := make(map[V]struct{}, c.Len())
	if len(c.order) > 0 {
		for _, id := range c.order {
			item, ok := c.itemsMap[id]
			if !ok {
				continue
			}
			v := yield(item)
			if skip != nil && skip(v) {
				continue
			}
			if _, exists := seen[v]; exists {
				continue
			}
			seen[v] = struct{}{}
			sl = append(sl, v)
		}
		return sl
	}
	// unordered
	for _, item := range c.itemsMap {
		v := yield(item)
		if skip != nil && skip(v) {
			continue
		}
		if _, exists := seen[v]; exists {
			continue
		}
		seen[v] = struct{}{}
		sl = append(sl, v)
	}
	return sl
}

// BuildUnorderedCollectionFrom constructs a new unordered collection
// by applying yield to each entity in the source collection.
// If yield returns nil, that entity is skipped.
// The resulting collection does not preserve iteration order.
func BuildUnorderedCollectionFrom[
	SP model.Identifiable[SID],
	SID comparable,
	NP model.Identifiable[NID],
	NID comparable,
](
	src *Collection[SP, SID],
	yield func(SP) NP,
	skip func(NP) bool,
) *Collection[NP, NID] {
	if src == nil {
		return nil
	}
	newColl := NewEmptyUnorderedCollection[NP, NID]()
	if skip != nil {
		for _, sp := range src.itemsMap {
			np := yield(sp)
			if !skip(np) {
				newColl.AddIfNew(np)
			}
		}
		return newColl
	}

	for _, sp := range src.itemsMap {
		np := yield(sp)
		newColl.AddIfNew(np)
	}
	return newColl
}

// BuildOrderedCollectionFrom constructs a new ordered collection
// by applying yield to each entity in the source collection.
// If yield returns nil, that entity is skipped.
// The resulting collection preserves the iteration order of src.
func BuildOrderedCollectionFrom[
	SP model.Identifiable[SID],
	SID comparable,
	NP model.Identifiable[NID],
	NID comparable,
](
	src *Collection[SP, SID],
	yield func(SP) NP,
	skip func(NP) bool,
) *Collection[NP, NID] {
	if src == nil {
		return nil
	}
	newColl := NewEmptyOrderedCollection[NP, NID]()

	if skip != nil {
		for _, id := range src.order {
			sp, ok := src.itemsMap[id]
			if !ok {
				continue
			}
			np := yield(sp)
			if !skip(np) {
				newColl.AddIfNew(np)
			}
		}
		return newColl
	}

	for _, id := range src.order {
		sp, ok := src.itemsMap[id]
		if !ok {
			continue
		}
		np := yield(sp)
		newColl.AddIfNew(np)
	}
	return newColl
}
