package kvdbs

import "errors"

type TTLState int

const (
	TTLKeyNotFound TTLState = iota + 1
	TTLPersistent
	TTLExpiring
)

var ErrNotSupported = errors.New("kvdbs: operation not supported")
