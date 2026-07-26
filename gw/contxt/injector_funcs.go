package contxt

import "context"

type UnaryInjectorFunc[T any] func(context.Context, T) (context.Context, error)

type BinaryInjectorFunc[T1, T2 any] func(context.Context, T1, T2) (context.Context, error)
