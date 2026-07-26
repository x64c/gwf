package sqldbs

import "github.com/x64c/gwf/gw/model"

type scanFieldsProvider interface {
	FieldsToScan() []any
}

type Scannable[T any] interface {
	Tabular[T] // ~*T + tableMetaProvider
	scanFieldsProvider
}

type ScannableIdentifiable[T any, ID comparable] interface {
	Scannable[T]
	model.Identifiable[ID]
}
