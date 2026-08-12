package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/model"
)

// The single-item DBModel relation loaders.

// LoadDBModelBelongsToOnItem - LoadDBModelBelongsTo for a single child item.
// Uses QueryDBModelFirst directly — no collection wrapping overhead.
// BelongsTo is strict: a missing parent surfaces as ErrNoRows.
// Writes the parent to *relationFieldPtr(child) and returns it.
func LoadDBModelBelongsToOnItem[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP IdentifiableDBModel[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	child CP,
	sqlSelectBase string,
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (PP, error) {
	var zero PP
	var zeroParent P
	pkCol, err := PP(&zeroParent).Table().SinglePKColumn()
	if err != nil {
		return zero, fmt.Errorf("LoadDBModelBelongsToOnItem: %w", err)
	}
	parent, err := QueryDBModelFirst[P, PP](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: BinPred{Column: pkCol, Op: OpEq, Value: foreignKey(child)},
	})
	if err != nil {
		return zero, err
	}
	*relationFieldPtr(child) = parent
	return parent, nil
}

// LoadDBModelHasManyOnItem - LoadDBModelHasMany for a single parent item.
// Uses QueryDBModelCollection — no parent-collection wrapping overhead and no
// grouping step (all queried children belong to the one parent by SQL filter).
// Writes the children collection to *relationFieldPtr(parent) and returns it.
func LoadDBModelHasManyOnItem[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parent PP,
	sqlSelectBase string,
	foreignKeyColumn Column,
	relationFieldPtr func(PP) **coll.Collection[CP, CID],
	orderBys ...OrderBy,
) (*coll.Collection[CP, CID], error) {
	children, err := QueryDBModelCollection[C, CP, CID](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: BinPred{Column: foreignKeyColumn, Op: OpEq, Value: parent.GetID()},
		OrderBys:  orderBys,
	})
	if err != nil {
		return nil, err
	}
	*relationFieldPtr(parent) = children
	return children, nil
}

// LoadDBModelHasManyQueryOptsOnItem - LoadDBModelHasManyQueryOpts for a single
// parent item. Merges queryOpts.WhereCond (if any) with the
// foreignKey=parent.ID predicate.
// Writes the children collection to *relationFieldPtr(parent) and returns it.
func LoadDBModelHasManyQueryOptsOnItem[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parent PP,
	sqlSelectBase string,
	foreignKeyColumn Column,
	relationFieldPtr func(PP) **coll.Collection[CP, CID],
	queryOpts QueryOpts,
) (*coll.Collection[CP, CID], error) {
	var cond Cond = BinPred{Column: foreignKeyColumn, Op: OpEq, Value: parent.GetID()}
	if queryOpts.WhereCond != nil {
		cond = And{Conds: []Cond{cond, queryOpts.WhereCond}}
	}
	children, err := QueryDBModelCollection[C, CP, CID](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: cond,
		OrderBys:  queryOpts.OrderBys,
	})
	if err != nil {
		return nil, err
	}
	*relationFieldPtr(parent) = children
	return children, nil
}

// LoadDBModelBelongsToOnItemWithStoreKey wraps LoadDBModelBelongsToOnItem
// with a RawSQLStore key lookup.
func LoadDBModelBelongsToOnItemWithStoreKey[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP IdentifiableDBModel[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	child CP,
	storeKey string,
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (PP, error) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		var zero PP
		return zero, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	return LoadDBModelBelongsToOnItem[CP, CID, P, PP, PID](ctx, db, child, sqlBase, foreignKey, relationFieldPtr)
}

// LoadDBModelHasManyOnItemWithStoreKey wraps LoadDBModelHasManyOnItem with a
// RawSQLStore key lookup and FK column name.
func LoadDBModelHasManyOnItemWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parent PP,
	storeKey string,
	fkColumnName string,
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
	return LoadDBModelHasManyOnItem[PP, PID, C, CP, CID](ctx, db, parent, sqlBase, fkCol, relationFieldPtr, orderBys...)
}

// LoadDBModelHasManyQueryOptsOnItemWithStoreKey wraps
// LoadDBModelHasManyQueryOptsOnItem with a RawSQLStore key lookup and FK
// column name.
func LoadDBModelHasManyQueryOptsOnItemWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP IdentifiableDBModel[C, CID],
	CID comparable,
](
	ctx context.Context,
	db DB,
	parent PP,
	storeKey string,
	fkColumnName string,
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
	return LoadDBModelHasManyQueryOptsOnItem[PP, PID, C, CP, CID](ctx, db, parent, sqlBase, fkCol, relationFieldPtr, queryOpts)
}
