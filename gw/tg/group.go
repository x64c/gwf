// Package tg provides TypedGroup: a typed, ID-keyed, order-preserving,
// boot-frozen collection. Apps hold each group in their own typed field
// (e.g. an LMS provider group) — the framework offers the type, not a home
// for the instances: a type-erased central registry could answer nothing
// typed about groups whose type parameters only the app can name.
package tg

import "fmt"

// TypedGroup is a generic, ID-keyed collection of items sharing the same type.
// Registration order is preserved. Populate at boot, read thereafter.
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

// Register adds an item under id, rejecting an empty or already-registered
// id by name. The refusal is what keeps the group's order (ids) and lookup
// (items) views agreeing: an overwrite would count twice and double in All
// while the first item silently vanished.
func (g *TypedGroup[T]) Register(id string, item T) error {
	if id == "" {
		return fmt.Errorf("tg: empty id")
	}
	if _, dup := g.items[id]; dup {
		return fmt.Errorf("tg: id %q already registered", id)
	}
	g.ids = append(g.ids, id)
	g.items[id] = item
	return nil
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
