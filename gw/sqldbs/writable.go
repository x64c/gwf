package sqldbs

import "github.com/x64c/gwf/gw/model"

// writeFieldsProvider provides non-PK columns and their values for writing (insert/update).
// PK is excluded — it's provided by TableMeta().PK + GetID().
type writeFieldsProvider interface {
	FieldsToWrite() map[string]any
}

type Writable[T any] interface {
	Tabular[T]
	writeFieldsProvider
}

// WritableIdentifiable = Writable + Identifiable. Conceptually: Updatable by PK.
type WritableIdentifiable[T any, ID comparable] interface {
	Writable[T]
	model.Identifiable[ID]
}
