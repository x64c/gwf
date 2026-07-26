package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/model"
	"github.com/x64c/gwf/gw/nullable"
)

// LoadBelongsTo - Load Parents on Children from SQL DB and Link Child-BelongsTo-Parent Relation
// Returns the Parents
func LoadBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any, // Model struct
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	sqlSelectBase string, // must be clean from WHERE and bindings
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	fKeysAsAny := coll.CollectUniqueToSlice(children, func(c CP) any { return foreignKey(c) })
	if len(fKeysAsAny) == 0 {
		return coll.NewEmptyOrderedCollection[PP, PID](), nil
	}
	sqlStmt := sqlSelectBase + fmt.Sprintf(" WHERE id IN (%s)", db.Client().InPlaceholders(1, len(fKeysAsAny)))
	parents, err := RawQueryCollection[P, PP, PID](ctx, db, sqlStmt, fKeysAsAny...)
	if err != nil {
		return nil, err
	}
	err = coll.LinkBelongsTo[CP, CID, PP, PID](children, parents, foreignKey, relationFieldPtr)
	if err != nil {
		return nil, err
	}
	return parents, nil
}

// LoadOptionalBelongsTo - Load Parents on Children from SQL DB and Link Child-BelongsTo-Parent Relation.
// Handles two cases:
//  1. FK (pointer to parent) in child is nil → skipped
//  2. Missing parent (child has FK but no matching parent in DB) → tolerant (allowed)
//
// In both cases the child's relation field is left nil; nil check required when accessing.
// Returns the Parent Collection.
func LoadOptionalBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any, // Model struct
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	sqlSelectBase string, // must be clean from WHERE and bindings
	foreignKeyFieldPtr func(c CP) *PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	fKeysAsAny := coll.CollectUniqueToSliceWithSkip(children,
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
	sqlStmt := sqlSelectBase + fmt.Sprintf(" WHERE id IN (%s)", db.Client().InPlaceholders(1, len(fKeysAsAny)))
	parents, err := RawQueryCollection[P, PP, PID](ctx, db, sqlStmt, fKeysAsAny...)
	if err != nil {
		return nil, err
	}
	coll.LinkOptionalBelongsTo[CP, CID, PP, PID](children, parents, foreignKeyFieldPtr, relationFieldPtr)
	return parents, nil
}

// LoadNullableBelongsTo - Convenience wrapper around LoadOptionalBelongsTo for nullable FK fields
// typed as nullable.Nullable[PID].
// Extracts the FK pointer via Ptr() and delegates.
// Returns the Parent Collection.
func LoadNullableBelongsTo[
	CP model.Identifiable[CID],
	CID comparable,
	P any, // Model struct
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	sqlSelectBase string, // must be clean from WHERE and bindings
	nullableFKField func(c CP) nullable.Nullable[PID],
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	return LoadOptionalBelongsTo[CP, CID, P, PP, PID](
		ctx, db, children, sqlSelectBase,
		func(c CP) *PID { return nullableFKField(c).Ptr() },
		relationFieldPtr,
	)
}

func LoadHasMany[
	PP model.Identifiable[PID],
	PID comparable,
	C any, // Model struct
	CP ScannableIdentifiable[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	sqlSelectBase string, // must be clean from WHERE and bindings
	foreignKeyColumn Column, // on the child
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **coll.Collection[CP, CID], // on the parent
	orderBys ...OrderBy,
) (*coll.Collection[CP, CID], error) {
	if parents.Len() == 0 {
		return coll.NewEmptyOrderedCollection[CP, CID](), nil
	}
	whereClause := fmt.Sprintf(" WHERE %s IN (%s)", foreignKeyColumn.Name(), db.Client().InPlaceholders(1, parents.Len()))
	sqlStmt := sqlSelectBase + whereClause + OrderByClause(orderBys)
	parentIDsAsAny := parents.IDsAsAny()
	children, err := RawQueryCollection[C, CP, CID](ctx, db, sqlStmt, parentIDsAsAny...)
	if err != nil {
		return nil, err
	}
	coll.LinkHasMany[PP, PID, CP, CID](
		parents,
		children,
		foreignKey,
		relationFieldPtr,
	)
	return children, nil
}

// LoadHasManyQueryOpts - Same as LoadHasMany but with QueryOpts for WHERE conditions and ORDER BY.
func LoadHasManyQueryOpts[
	PP model.Identifiable[PID],
	PID comparable,
	C any, // Model struct
	CP ScannableIdentifiable[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	sqlSelectBase string, // must be clean from WHERE and bindings
	foreignKeyColumn Column, // on the child
	foreignKey func(CP) PID, // on the child
	relationFieldPtr func(PP) **coll.Collection[CP, CID], // on the parent
	queryOpts QueryOpts,
) (*coll.Collection[CP, CID], error) {
	if parents.Len() == 0 {
		return coll.NewEmptyOrderedCollection[CP, CID](), nil
	}
	var cond Cond = InPred{Column: foreignKeyColumn, Values: parents.IDsAsAny()}
	if queryOpts.WhereCond != nil {
		cond = And{Conds: []Cond{cond, queryOpts.WhereCond}}
	}
	whereSQL, args := WhereClause{cond}.Build(db.Client(), 1)
	sqlStmt := sqlSelectBase + whereSQL + OrderByClause(queryOpts.OrderBys)
	children, err := RawQueryCollection[C, CP, CID](ctx, db, sqlStmt, args...)
	if err != nil {
		return nil, err
	}
	coll.LinkHasMany[PP, PID, CP, CID](
		parents,
		children,
		foreignKey,
		relationFieldPtr,
	)
	return children, nil
}

// LoadBelongsToWithStoreKey wraps LoadBelongsTo with a RawSQLStore key lookup.
func LoadBelongsToWithStoreKey[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	storeKey string,
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	return LoadBelongsTo[CP, CID, P, PP, PID](ctx, db, children, sqlBase, foreignKey, relationFieldPtr)
}

// LoadOptionalBelongsToWithStoreKey wraps LoadOptionalBelongsTo with a RawSQLStore key lookup.
func LoadOptionalBelongsToWithStoreKey[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	storeKey string,
	foreignKeyFieldPtr func(c CP) *PID,
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	return LoadOptionalBelongsTo[CP, CID, P, PP, PID](ctx, db, children, sqlBase, foreignKeyFieldPtr, relationFieldPtr)
}

// LoadNullableBelongsToWithStoreKey wraps LoadNullableBelongsTo with a RawSQLStore key lookup.
func LoadNullableBelongsToWithStoreKey[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	children *coll.Collection[CP, CID],
	storeKey string,
	nullableFKField func(c CP) nullable.Nullable[PID],
	relationFieldPtr func(c CP) *PP,
) (
	*coll.Collection[PP, PID],
	error,
) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	return LoadNullableBelongsTo[CP, CID, P, PP, PID](ctx, db, children, sqlBase, nullableFKField, relationFieldPtr)
}

// LoadHasManyWithStoreKey wraps LoadHasMany with a RawSQLStore key lookup and FK column name.
func LoadHasManyWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	storeKey string,
	fkColumnName string,
	foreignKey func(CP) PID,
	relationFieldPtr func(PP) **coll.Collection[CP, CID],
	orderBys ...OrderBy,
) (*coll.Collection[CP, CID], error) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	fkCol, err := NewColumn(fkColumnName)
	if err != nil {
		return nil, fmt.Errorf("invalid foreign key column name %q", fkColumnName)
	}
	return LoadHasMany[PP, PID, C, CP, CID](ctx, db, parents, sqlBase, fkCol, foreignKey, relationFieldPtr, orderBys...)
}

// LoadHasManyQueryOptsWithStoreKey wraps LoadHasManyQueryOpts with a RawSQLStore key lookup and FK column name.
func LoadHasManyQueryOptsWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parents *coll.Collection[PP, PID],
	storeKey string,
	fkColumnName string,
	foreignKey func(CP) PID,
	relationFieldPtr func(PP) **coll.Collection[CP, CID],
	queryOpts QueryOpts,
) (*coll.Collection[CP, CID], error) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	fkCol, err := NewColumn(fkColumnName)
	if err != nil {
		return nil, fmt.Errorf("invalid foreign key column name %q", fkColumnName)
	}
	return LoadHasManyQueryOpts[PP, PID, C, CP, CID](ctx, db, parents, sqlBase, fkCol, foreignKey, relationFieldPtr, queryOpts)
}
