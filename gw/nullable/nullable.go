package nullable

import "encoding/json/v2"

// Nullable is the interface that all nullable types satisfy.
type Nullable[T any] interface {
	Ptr() *T
	ForceValue() T
	IsNil() bool
	json.Marshaler
	json.Unmarshaler
}
