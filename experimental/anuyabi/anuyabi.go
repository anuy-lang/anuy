// Package anuyabi is the experimental ABI support package (RFC-009
// §6.1.5–6.1.9 sketch): stable representation helpers for generated Go -
// no runtime, no scheduler, no hidden state. This validation copy lives
// in experimental/ until RFC-009 is accepted; the physical import path
// and release form belong to RFC-010.
package anuyabi

// Nullable is the canonical tagged nullable of the ABI v1 sketch
// (RFC-009 §6.1.7): Present == false is semantic nil (§6.1.8) and the
// zero value represents nil (§6.1.9). With one presence tag plus a value
// payload, every field combination is a valid state - there is no
// invalid tag combination (RFC-002 §6.9.6).
type Nullable[T any] struct {
	Value   T
	Present bool
}

// Some wraps a present value.
func Some[T any](value T) Nullable[T] {
	return Nullable[T]{Value: value, Present: true}
}

// None returns the nil carrier.
func None[T any]() Nullable[T] {
	return Nullable[T]{}
}

// Get reports the carried value and its presence.
func (n Nullable[T]) Get() (T, bool) {
	return n.Value, n.Present
}

// IsNil reports semantic nil.
func (n Nullable[T]) IsNil() bool {
	return !n.Present
}
