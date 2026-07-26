package kvdbs

import "encoding/json/jsontext"

type Client interface {
	CreateDB(name string, conf jsontext.Value) error
	DB(name string) (DB, bool)
	Close() error
}
