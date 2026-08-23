package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/model"
	"github.com/x64c/gwf/gw/nullable"
)

// The collection DBModel relation loaders.

// LoadDBModelBelongsTo - Load Parents on Children from SQL DB and Link
// Child-BelongsTo-Parent Relation. Returns the Parents.
func LoadDBModelBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP IdentifiableDBModel[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	fKeysAsAny := children.CollectUniqueToSlice(func(c CP) any { return foreignKey(c) })
	if len(fKeysAsAny) == 0 {
		return coll.NewEmptyOrderedCollection[PP, PID](), nil
	}
	var zeroParent P
	pkCol, err := PP(&zeroParent).Table().SinglePKColumn()
	if err != nil {
		return nil, fmt.Errorf("LoadDBModelBelongsTo: %w", err)
	}
	parents, err := QueryDBModelCollectionByColumn[P, PP, PID, any](ctx, db, pkCol, fKeysAsAny)
	if err != nil {
		return nil, err
	}
	err = children.LinkBelongsTo(parents, foreignKey, relationFieldPtr)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

// LoadDBModelOptionalBelongsTo - Load Parents on Children from SQL DB and Link
// Child-BelongsTo-Parent Relation. Handles two cases:
//  1. FK (pointer to parent) in child is nil → skipped
//  2. Missing parent (child has FK but no matching parent in DB) → tolerant (allowed)
//
// In both cases the child's relation field is left nil; nil check required
// when accessing. Returns the Parent Collection.
func LoadDBModelOptionalBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP IdentifiableDBModel[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	foreignKeyFieldPtr func(c CP) *PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	fKeysAsAny := children.CollectUniqueToSliceWithSkip(
		func(c CP) any {
			ptr := foreignKeyFieldPtr(c)
			if ptr == nil {
				return nil
			}
			return *ptr
		},
		func(v any) bool { return v == nil },
	)
	if len(fKeysAsAny) == 0 {
		return coll.NewEmptyOrderedCollection[PP, PID](), nil
	}
	var zeroParent P
	pkCol, err := PP(&zeroParent).Table().SinglePKColumn()
	if err != nil {
		return nil, fmt.Errorf("LoadDBModelOptionalBelongsTo: %w", err)
	}
	parents, err := QueryDBModelCollectionByColumn[P, PP, PID, any](ctx, db, pkCol, fKeysAsAny)
	if err != nil {
		return nil, err
	}
	children.LinkOptionalBelongsTo(parents, foreignKeyFieldPtr, relationFieldPtr)
	return parents, nil
}

// LoadDBModelNullableBelongsTo - Convenience wrapper around
// LoadDBModelOptionalBelongsTo for nullable FK fields typed as
// nullable.Nullable[PID]. Extracts the FK pointer via Ptr() and delegates.
// Returns the Parent Collection.
func LoadDBModelNullableBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP IdentifiableDBModel[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	nullableFKField func(c CP) nullable.Nullable[PID],
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	return LoadDBModelOptionalBelongsTo[CP, CID, P, PP, PID](
		ctx, db, children,
		func(c CP) *PID { return nullableFKField(c).Ptr() },
		relationFieldPtr,
	)
}

func LoadDBModelHasMany[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	foreignKeyColumn Column, // on the child
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **coll.Collection[CP, CID], // on the parent
	orderBys ...OrderBy,
) (*coll.Collection[CP, CID], error) {
	if parents.Len() == 0 {
		return coll.NewEmptyOrderedCollection[CP, CID](), nil
	}
	sqlSelectBase, err := SelectBaseOf[C, CP](db.Client())
	if err != nil {
		return nil, fmt.Errorf("LoadDBModelHasMany: %w", err)
	}
	whereClause := fmt.Sprintf(" WHERE %s IN (%s)", foreignKeyColumn.Name(), db.Client().InPlaceholders(1, parents.Len()))
	sqlStmt := sqlSelectBase + whereClause + OrderByClause(orderBys)
	parentIDsAsAny := parents.IDsAsAny()
	children, err := RawQueryDBModelCollection[C, CP, CID](ctx, db, sqlStmt, parentIDsAsAny...)
	if err != nil {
		return nil, err
	}
	parents.LinkHasMany(children, foreignKey, relationFieldPtr)
	return children, nil
}

// LoadDBModelHasManyQueryOpts - Same as LoadDBModelHasMany but with QueryOpts
// for WHERE conditions and ORDER BY.
func LoadDBModelHasManyQueryOpts[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	foreignKeyColumn Column, // on the child
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **coll.Collection[CP, CID], // on the parent
	queryOpts QueryOpts,
) (*coll.Collection[CP, CID], error) {
	if parents.Len() == 0 {
		return coll.NewEmptyOrderedCollection[CP, CID](), nil
	}
	sqlSelectBase, err := SelectBaseOf[C, CP](db.Client())
	if err != nil {
		return nil, fmt.Errorf("LoadDBModelHasManyQueryOpts: %w", err)
	}
	var cond Cond = InPred{Column: foreignKeyColumn, Values: parents.IDsAsAny()}
	if queryOpts.WhereCond != nil {
		cond = And{Conds: []Cond{cond, queryOpts.WhereCond}}
	}
	whereSQL, args := WhereClause{cond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys)
	children, err := RawQueryDBModelCollection[C, CP, CID](ctx, db, sqlStmt, args...)
	if err != nil {
		return nil, err
	}
	parents.LinkHasMany(children, foreignKey, relationFieldPtr)
	return children, nil
}
