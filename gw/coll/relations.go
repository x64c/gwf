package coll

import (
	"fmt"

	"github.com/x64c/gwf/gw/model"
)

// LinkOptionalBelongsTo connects ChildCollection-ParentCollection where Child-BelongsTo-Parent
// ForeignKeyField is on the Child
// RelationField is on the Child
// Optional Version
func (children *Collection[CP, CID]) LinkOptionalBelongsTo[
	PP model.Identifiable[PID],
	PID comparable,
](
	parents *Collection[PP, PID],
	foreignKeyFieldPtr func(CP) *PID, // on the child
	relationFieldPtr func(CP) *PP, // on the child
) {
	for _, child := range children.itemsMap {
		fkPtr := foreignKeyFieldPtr(child)
		if fkPtr == nil {
			continue
		}
		fk := *fkPtr
		if parent, ok := parents.itemsMap[fk]; ok {
			*relationFieldPtr(child) = parent
		}
	}
}

// LinkBelongsTo - Strict Version
// ForeignKeyField is on the Child
// RelationField is on the Child
func (children *Collection[CP, CID]) LinkBelongsTo[
	PP model.Identifiable[PID],
	PID comparable,
](
	parents *Collection[PP, PID],
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(CP) *PP, // on the child
) error {
	for _, child := range children.itemsMap {
		fk := foreignKey(child)
		parent, ok := parents.itemsMap[fk]
		if !ok {
			return fmt.Errorf(
				"LinkBelongsTo: parent with ID %v not found for child ID %v",
				fk, child.GetID(),
			)
		}
		*relationFieldPtr(child) = parent
	}
	return nil
}

// LinkHasMany wires each parent's HasMany relation field to its children: the
// children are grouped by foreignKey (their parent's ID), and each parent gets
// its own child collection — an empty one when it has none. Each side keeps its
// kind: parents is only read, so its kind and order stay as they are; every
// child collection takes the children's kind — ordered children give ordered
// child collections, each in the children's relative order (e.g. a query's
// ORDER BY); unordered children give unordered ones.
func (parents *Collection[PP, PID]) LinkHasMany[
	CP model.Identifiable[CID],
	CID comparable,
](
	children *Collection[CP, CID],
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **Collection[CP, CID], // on the parent
) {
	var childCollCtor func() *Collection[CP, CID]
	if children.IsOrdered() {
		childCollCtor = NewEmptyOrderedCollection[CP, CID]
	} else {
		childCollCtor = NewEmptyUnorderedCollection[CP, CID]
	}
	childCollGrpByPID := make(map[PID]*Collection[CP, CID], parents.Len())
	children.ForEach(func(child CP) { // the children's order when ordered
		pid := foreignKey(child)
		childColl, ok := childCollGrpByPID[pid]
		if !ok {
			childColl = childCollCtor()
			childCollGrpByPID[pid] = childColl
		}
		childColl.Add(child)
	})
	for pid, parent := range parents.itemsMap {
		if childColl, ok := childCollGrpByPID[pid]; ok {
			*relationFieldPtr(parent) = childColl
		} else {
			*relationFieldPtr(parent) = childCollCtor()
		}
	}
}
