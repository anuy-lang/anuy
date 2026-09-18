package semantic

// Evidence model from the kernel validation: the early tagged carrier
// that predates the generated nullable prelude (RFC-002 §6.7–6.8,
// RFC-009 §6.1.7). Not wired into the flow layer; kept as validation
// evidence.

// Nullable is an experimental tagged representation for nullable value types.
type Nullable[T any] struct {
	Value   T
	Present bool
}

// Present wraps a value into a present carrier.
func Present[T any](value T) Nullable[T] { return Nullable[T]{Value: value, Present: true} }

// Nil returns the nil carrier: the zero value is semantic nil.
func Nil[T any]() Nullable[T] { return Nullable[T]{} }

// Get reports the value and its presence.
func (value Nullable[T]) Get() (T, bool) { return value.Value, value.Present }
