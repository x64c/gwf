package storages

import "encoding/json/jsontext"

type Client interface {
	CreateStorage(name string, conf jsontext.Value) error
	Storage(name string) (Storage, bool)
}
