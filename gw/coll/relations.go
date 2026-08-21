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

// LinkHasMany connects ParentCollection-ChildCollection where a Parent-HasMany-Children
// ForeignKeyField is on the Child
// RelationField (a Slice) is on the Parent
func (parents *Collection[PP, PID]) LinkHasMany[
	CP model.Identifiable[CID],
	CID comparable,
](
	children *Collection[CP, CID],
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **Collection[CP, CID], // on the parent
) {
	childCollGrpByPID := make(map[PID]*Collection[CP, CID], parents.Len())
	if len(children.order) > 0 {
		// Ordered: iterate in order to preserve SQL ORDER BY
		for _, cid := range children.order {
			child := children.itemsMap[cid]
			pid := foreignKey(child)
			childColl, ok := childCollGrpByPID[pid]
			if !ok {
				childColl = NewEmptyOrderedCollection[CP, CID]()
				childCollGrpByPID[pid] = childColl
			}
			childColl.Add(child)
		}
	} else {
		for _, child := range children.itemsMap {
			pid := foreignKey(child)
			childColl, ok := childCollGrpByPID[pid]
			if !ok {
				childColl = NewEmptyOrderedCollection[CP, CID]()
				childCollGrpByPID[pid] = childColl
			}
			childColl.Add(child)
		}
	}
	for pid, parent := range parents.itemsMap {
		if childColl, ok := childCollGrpByPID[pid]; ok {
			*relationFieldPtr(parent) = childColl
		} else {
			*relationFieldPtr(parent) = NewEmptyOrderedCollection[CP, CID]()
		}
	}
}
