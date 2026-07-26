package coll

import (
	"encoding/json/v2"
	"fmt"
	"iter"
	"sort"

	"github.com/x64c/gwf/gw/model"
)

// Collection (orm.Collection) is a set-like container of identifiable entities.
// It enforces uniqueness by ID and optionally preserves iteration order
type Collection[MP model.Identifiable[ID], ID comparable] struct {
	itemsMap map[ID]MP // uniqueness enforced by ID
	order    []ID      // optional (default = nil). only if you care about iteration order
}

func NewEmptyOrderedCollection[
	MP model.Identifiable[ID],
	ID comparable,
]() *Collection[MP, ID] {
	return &Collection[MP, ID]{
		itemsMap: make(map[ID]MP),
		order:    make([]ID, 0),
	}
}

func NewEmptyUnorderedCollection[
	MP model.Identifiable[ID],
	ID comparable,
]() *Collection[MP, ID] {
	return &Collection[MP, ID]{
		itemsMap: make(map[ID]MP),
	}
}

func NewUnorderedCollection[
	MP model.Identifiable[ID],
	ID comparable,
](items []MP) *Collection[MP, ID] {
	coll := &Collection[MP, ID]{
		itemsMap: make(map[ID]MP, len(items)),
	}
	for _, item := range items {
		coll.itemsMap[item.GetID()] = item
	}
	return coll
}

func NewOrderedCollection[
	MP model.Identifiable[ID],
	ID comparable,
](items []MP) *Collection[MP, ID] {
	coll := &Collection[MP, ID]{
		itemsMap: make(map[ID]MP, len(items)),
		order:    make([]ID, len(items)),
	}
	for i, item := range items {
		id := item.GetID()
		coll.itemsMap[id] = item
		coll.order[i] = id
	}
	return coll
}

func (c *Collection[MP, ID]) Len() int {
	return len(c.itemsMap)
}

func (c *Collection[MP, ID]) IsOrdered() bool {
	return c.order != nil
}

func (c *Collection[MP, ID]) RemoveOrder() {
	c.order = nil
}

func (c *Collection[MP, ID]) Sort(less func(MP, MP) bool) { // if less(a,b) == true -> a comes before b
	if c.order == nil {
		ids := make([]ID, 0, len(c.itemsMap))
		for id := range c.itemsMap {
			ids = append(ids, id)
		}
		c.order = ids
	}
	sort.SliceStable(c.order, func(i, j int) bool { // i, j are indices
		return less(c.itemsMap[c.order[i]], c.itemsMap[c.order[j]])
	})
}

func (c *Collection[MP, ID]) Has(id ID) bool {
	_, ok := c.itemsMap[id]
	return ok
}

func (c *Collection[MP, ID]) Find(id ID) (MP, bool) {
	p, ok := c.itemsMap[id]
	return p, ok
}

func (c *Collection[MP, ID]) Add(item MP) {
	id := item.GetID()
	_, exists := c.itemsMap[id]
	c.itemsMap[id] = item
	// Preserve order if ordered collection
	if c.order != nil && !exists {
		c.order = append(c.order, id)
	}
}

func (c *Collection[MP, ID]) AddIfNew(item MP) {
	id := item.GetID()
	if _, exists := c.itemsMap[id]; exists {
		return
	}
	c.itemsMap[id] = item
	// Preserve order if ordered collection
	if c.order != nil {
		c.order = append(c.order, id)
	}
}

func (c *Collection[MP, ID]) IDs() []ID {
	if c.order != nil {
		return append([]ID(nil), c.order...) // preserve original order
	}
	ids := make([]ID, 0, len(c.itemsMap))
	for id := range c.itemsMap {
		ids = append(ids, id)
	}
	return ids
}

func (c *Collection[MP, ID]) IDsAsAny() []any {
	if c.order != nil {
		ids := make([]any, len(c.order))
		for i, id := range c.order {
			ids[i] = id
		}
		return ids
	}
	ids := make([]any, 0, len(c.itemsMap))
	for id := range c.itemsMap {
		ids = append(ids, id)
	}
	return ids
}

func (c *Collection[MP, ID]) Items() []MP {
	if c.order != nil {
		items := make([]MP, 0, len(c.order))
		for _, id := range c.order {
			items = append(items, c.itemsMap[id])
		}
		return items
	}
	items := make([]MP, 0, len(c.itemsMap))
	for _, item := range c.itemsMap {
		items = append(items, item)
	}
	return items
}

func (c *Collection[MP, ID]) MarshalJSON() ([]byte, error) {
	if c == nil {
		return []byte("null"), nil
	}
	return json.Marshal(c.Items())
}

// ForEach calls fn for every model in the collection.
// If the collection has an order, it respects that order.
// Has NO way to exit out of the whole iteration — always loops through all items.
// For early exit, use Iter() with range-over-func + break.
func (c *Collection[MP, ID]) ForEach(fn func(MP)) {
	if c.order != nil {
		for _, id := range c.order {
			if mp, ok := c.itemsMap[id]; ok {
				fn(mp)
			}
		}
		return
	}
	for _, mp := range c.itemsMap {
		fn(mp)
	}
}

func (c *Collection[MP, ID]) ForEachUnorderly(fn func(MP)) {
	for _, mp := range c.itemsMap {
		fn(mp)
	}
}

func (c *Collection[MP, ID]) ForEachOrderly(fn func(MP)) error {
	if len(c.order) == 0 {
		return fmt.Errorf("collection is unordered")
	}
	for _, id := range c.order {
		if mp, ok := c.itemsMap[id]; ok {
			fn(mp)
		}
	}
	return nil
}

// Iter returns an iterator over the collection's items.
// If ordered, respects order.
// Supports break/continue naturally — break actually stops the producer,
// not just the consumer loop. Use when early exit is needed.
// For full iteration without early exit, ForEach is slightly cheaper (no yield bool check).
func (c *Collection[MP, ID]) Iter() iter.Seq[MP] {
	return func(yield func(MP) bool) {
		if c.order != nil {
			for _, id := range c.order {
				if mp, ok := c.itemsMap[id]; ok {
					if !yield(mp) {
						return
					}
				}
			}
		} else {
			for _, mp := range c.itemsMap {
				if !yield(mp) {
					return
				}
			}
		}
	}
}

func (c *Collection[MP, ID]) Filter(fn func(MP) bool) *Collection[MP, ID] {
	// If ordered, keep the same order slice layout
	if len(c.order) > 0 {
		filtered := &Collection[MP, ID]{
			itemsMap: make(map[ID]MP, len(c.itemsMap)),
			order:    make([]ID, 0, len(c.order)),
		}
		for _, id := range c.order {
			item := c.itemsMap[id]
			if fn(item) {
				filtered.itemsMap[id] = item
				filtered.order = append(filtered.order, id)
			}
		}
		return filtered
	}
	// Unordered — iterate directly over the map
	filtered := &Collection[MP, ID]{
		itemsMap: make(map[ID]MP, len(c.itemsMap)),
	}
	for id, item := range c.itemsMap {
		if fn(item) {
			filtered.itemsMap[id] = item
		}
	}
	return filtered
}

// First returns the first item from the collection if ordered.
// If unordered, the returned item is non-deterministic.
func (c *Collection[MP, ID]) First() (MP, bool) {
	if c.order != nil && len(c.order) > 0 {
		return c.itemsMap[c.order[0]], true
	}
	for _, mp := range c.itemsMap {
		return mp, true
	}
	var zero MP
	return zero, false
}

// FirstBy returns the first item matching the predicate.
// If ordered, respects order. If unordered, non-deterministic.
func (c *Collection[MP, ID]) FirstBy(fn func(MP) bool) (MP, bool) {
	if c.order != nil {
		for _, id := range c.order {
			if mp, ok := c.itemsMap[id]; ok && fn(mp) {
				return mp, true
			}
		}
	} else {
		for _, mp := range c.itemsMap {
			if fn(mp) {
				return mp, true
			}
		}
	}
	var zero MP
	return zero, false
}
