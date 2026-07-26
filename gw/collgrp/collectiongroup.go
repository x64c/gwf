package collgrp

import (
	"encoding/json/v2"
	"sort"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/model"
)

type CollectionGroup[
	MP model.Identifiable[ID],
	ID comparable,
	K comparable,
] struct {
	collectionsMap map[K]*coll.Collection[MP, ID]
	order          []K // optional (default = nil). only if you care about group order
}

func NewEmptyCollectionGroup[
	MP model.Identifiable[ID],
	ID comparable,
	K comparable,
]() *CollectionGroup[MP, ID, K] {
	return &CollectionGroup[MP, ID, K]{
		collectionsMap: make(map[K]*coll.Collection[MP, ID]),
	}
}

func (g *CollectionGroup[MP, ID, K]) FindCollection(key K) (*coll.Collection[MP, ID], bool) {
	c, ok := g.collectionsMap[key]
	return c, ok
}

func (g *CollectionGroup[MP, ID, K]) SetCollection(key K, c *coll.Collection[MP, ID]) {
	g.collectionsMap[key] = c
}

func (g *CollectionGroup[MP, ID, K]) Sort(less func(K, K) bool) {
	if g.order == nil {
		keys := make([]K, 0, len(g.collectionsMap))
		for k := range g.collectionsMap {
			keys = append(keys, k)
		}
		g.order = keys
	}
	sort.SliceStable(g.order, func(i, j int) bool {
		return less(g.order[i], g.order[j])
	})
}

func (g *CollectionGroup[MP, ID, K]) Collections() []*coll.Collection[MP, ID] {
	if g.order != nil {
		colls := make([]*coll.Collection[MP, ID], 0, len(g.order))
		for _, k := range g.order {
			colls = append(colls, g.collectionsMap[k])
		}
		return colls
	}
	colls := make([]*coll.Collection[MP, ID], 0, len(g.collectionsMap))
	for _, c := range g.collectionsMap {
		colls = append(colls, c)
	}
	return colls
}

func (g *CollectionGroup[MP, ID, K]) CollectionsMap() map[K]*coll.Collection[MP, ID] {
	return g.collectionsMap
}

func (g *CollectionGroup[MP, ID, K]) Keys() []K {
	if g.order != nil {
		return append([]K(nil), g.order...) // preserve original group order
	}
	keys := make([]K, 0, len(g.collectionsMap))
	for key := range g.collectionsMap {
		keys = append(keys, key)
	}
	return keys
}

func (g *CollectionGroup[MP, ID, K]) KeysAsAny() []any {
	if g.order != nil {
		keys := make([]any, len(g.order))
		for i, k := range g.order {
			keys[i] = k
		}
		return keys
	}
	keys := make([]any, 0, len(g.collectionsMap))
	for k := range g.collectionsMap {
		keys = append(keys, k)
	}
	return keys
}

func (g *CollectionGroup[MP, ID, K]) MarshalJSON() ([]byte, error) {
	if g == nil {
		return []byte("null"), nil
	}
	if g.order == nil {
		return json.Marshal(g.collectionsMap)
	}
	return json.Marshal(map[string]any{
		"order": g.order,
		"map":   g.collectionsMap,
	})
}
