package sqldbs

type tableMetaProvider interface {
	TableMeta() *TableMeta
}

type Tabular[T any] interface {
	~*T
	tableMetaProvider
}
