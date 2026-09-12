package semantic

// Nullable is an experimental tagged representation for nullable value types.
type Nullable[T any] struct {
	Value   T
	Present bool
}

func Present[T any](value T) Nullable[T] { return Nullable[T]{Value: value, Present: true} }
func Nil[T any]() Nullable[T]            { return Nullable[T]{} }
func (value Nullable[T]) Get() (T, bool) { return value.Value, value.Present }
