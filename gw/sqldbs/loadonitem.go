package sqldbs

import (
	"context"
	"fmt"

	"github.com/x64c/gwf/gw/coll"
	"github.com/x64c/gwf/gw/errs"
	"github.com/x64c/gwf/gw/model"
)

// LoadBelongsToOnItem - LoadBelongsTo for a single child item.
// Uses QueryFirst directly — no collection wrapping overhead.
// BelongsTo is strict: a missing parent surfaces as ErrNoRows from QueryFirst.
// Writes the parent to *relationFieldPtr(child) and returns it.
func LoadBelongsToOnItem[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	child CP,
	sqlSelectBase string,
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (*P, error) {
	var zero P
	pkCol, _ := NewColumn(PP(&zero).TableMeta().PK)
	parent, err := QueryFirst[P, PP](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: BinPred{Column: pkCol, Op: OpEq, Value: foreignKey(child)},
	})
	if err != nil {
		return nil, err
	}
	*relationFieldPtr(child) = PP(parent)
	return parent, nil
}

// LoadHasManyOnItem - LoadHasMany for a single parent item.
// Uses QueryCollection — no parent-collection wrapping overhead and no grouping step
// (all queried children belong to the one parent by SQL filter).
// Writes the children collection to *relationFieldPtr(parent) and returns it.
func LoadHasManyOnItem[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
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
	children, err := QueryCollection[C, CP, CID](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: BinPred{Column: foreignKeyColumn, Op: OpEq, Value: parent.GetID()},
		OrderBys:  orderBys,
	})
	if err != nil {
		return nil, err
	}
	*relationFieldPtr(parent) = children
	return children, nil
}

// LoadHasManyQueryOptsOnItem - LoadHasManyQueryOpts for a single parent item.
// Merges queryOpts.WhereCond (if any) with the foreignKey=parent.ID predicate.
// Writes the children collection to *relationFieldPtr(parent) and returns it.
func LoadHasManyQueryOptsOnItem[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
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
	children, err := QueryCollection[C, CP, CID](ctx, db, sqlSelectBase, QueryOpts{
		WhereCond: cond,
		OrderBys:  queryOpts.OrderBys,
	})
	if err != nil {
		return nil, err
	}
	*relationFieldPtr(parent) = children
	return children, nil
}

// LoadBelongsToOnItemWithStoreKey wraps LoadBelongsToOnItem with a RawSQLStore key lookup.
func LoadBelongsToOnItemWithStoreKey[
	CP model.Identifiable[CID],
	CID comparable,
	P any,
	PP ScannableIdentifiable[P, PID],
	PID comparable,
](
	ctx context.Context,
	db DB,
	child CP,
	storeKey string,
	foreignKey func(c CP) PID,
	relationFieldPtr func(c CP) *PP,
) (*P, error) {
	sqlBase, ok := db.MainRawSQLStore().Get(storeKey)
	if !ok {
		return nil, errs.SQLNotFoundInStore.WithDetail(storeKey)
	}
	return LoadBelongsToOnItem[CP, CID, P, PP, PID](ctx, db, child, sqlBase, foreignKey, relationFieldPtr)
}

// LoadHasManyOnItemWithStoreKey wraps LoadHasManyOnItem with a RawSQLStore key lookup and FK column name.
func LoadHasManyOnItemWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
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
	return LoadHasManyOnItem[PP, PID, C, CP, CID](ctx, db, parent, sqlBase, fkCol, relationFieldPtr, orderBys...)
}

// LoadHasManyQueryOptsOnItemWithStoreKey wraps LoadHasManyQueryOptsOnItem with a RawSQLStore key lookup and FK column name.
func LoadHasManyQueryOptsOnItemWithStoreKey[
	PP model.Identifiable[PID],
	PID comparable,
	C any,
	CP ScannableIdentifiable[C, CID],
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
	return LoadHasManyQueryOptsOnItem[PP, PID, C, CP, CID](ctx, db, parent, sqlBase, fkCol, relationFieldPtr, queryOpts)
}
