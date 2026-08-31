package kvdbs

import "encoding/json/jsontext"

type Client interface {
	// PrepareDB makes the database described by conf ready for use and
	// registers it under name — the database itself already exists; this
	// builds the handle that reaches it.
	PrepareDB(name string, conf jsontext.Value) error
	DB(name string) (DB, bool)
	Close() error
}
