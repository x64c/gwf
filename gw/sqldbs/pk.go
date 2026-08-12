package sqldbs

// PK is a primary key value: one value per key column, in the Table's
// key-column order.
//
// It is the only form a row's identity takes at this layer, whatever the key's
// shape — PK{v} for a single-column key, PK{v1, v2} for a key spanning two.
// One spelling for every case means nothing here is overloaded: the values are
// positional, the nth belonging to the nth key column, and length is the whole
// validity rule. An implementation MUST reject, before executing, a PK whose
// length differs from the table's key column count — an empty PK therefore
// matches no table.
//
// Vocabulary, package-wide: "id" names the fact of which row. It takes
// exactly two forms. The model's form is one comparable value — GetID() ID —
// the program's bookkeeping shape; for a single-column key it IS the key's
// bare value, and the SinglePK conveniences accept it as id and wrap it. The
// database's form is PK, and a parameter carrying it is named pkValue: on
// reading one, write PK{...}.
type PK []any
