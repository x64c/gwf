package sqldbs

import "github.com/x64c/gwf/gw/model"

// tableProvider provides the table a model is wired to.
type tableProvider interface {
	Table() *Table
}

// DBModel is a model backed by a SQL DB: wired to a table, its fields bound
// to columns. One interface serves both directions — reading a row into the
// model and writing the model as a row — with the write set derived from the
// bindings rather than declared separately. Whether a model is only ever read
// is a fact about its use, not its type.
type DBModel[T any] interface {
	~*T
	tableProvider
	columnFieldBindingsProvider
}

// IdentifiableDBModel is a DBModel that also identifies its rows — what the
// by-key model operations require.
type IdentifiableDBModel[T any, ID comparable] interface {
	DBModel[T]
	model.Identifiable[ID]
}
