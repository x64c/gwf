// Package tg provides a typed group registry.
// It allows registering and retrieving typed items by string ID,
// grouped under a common interface.
package tg

// RegGrp is the non-generic interface that all typed groups satisfy.
// This allows storing different TypedGroup[T] instances in a single map.
type RegGrp interface {
	Has(id string) bool
	Len() int
	IDs() []string
}

// TypedGroup is a generic, ID-keyed collection of items sharing the same type.
// Registration order is preserved.
type TypedGroup[T any] struct {
	ids   []string
	items map[string]T
}

// NewTypedGroup creates a new empty TypedGroup.
func NewTypedGroup[T any]() *TypedGroup[T] {
	return &TypedGroup[T]{
		items: make(map[string]T),
	}
}

// Register adds an item to the group with the given ID.
func (g *TypedGroup[T]) Register(id string, item T) {
	g.ids = append(g.ids, id)
	g.items[id] = item
}

// Get retrieves an item by ID.
func (g *TypedGroup[T]) Get(id string) (T, bool) {
	item, ok := g.items[id]
	return item, ok
}

// All returns all items in registration order.
func (g *TypedGroup[T]) All() []T {
	all := make([]T, len(g.ids))
	for i, id := range g.ids {
		all[i] = g.items[id]
	}
	return all
}

// Has reports whether an item with the given ID exists.
func (g *TypedGroup[T]) Has(id string) bool {
	_, ok := g.items[id]
	return ok
}

// Len returns the number of items in the group.
func (g *TypedGroup[T]) Len() int {
	return len(g.ids)
}

// IDs returns all registered IDs in registration order.
func (g *TypedGroup[T]) IDs() []string {
	return g.ids
}

// From extracts a *TypedGroup[T] from a RegGrp interface.
// Intended for one-time use at boot to create typed shortcuts.
func From[T any](g RegGrp) *TypedGroup[T] {
	return g.(*TypedGroup[T])
}
