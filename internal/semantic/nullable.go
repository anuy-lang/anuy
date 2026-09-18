// Evidence model from the kernel validation: the early value model behind
// Widen/Narrow. It is not wired into the flow-facts layer and is kept as
// validation evidence; its wiring point is RFC-002 §6.4 navigation.
package semantic

import "errors"

// Type pairs a named type with its declared nullability.
type Type struct {
	Name     string
	Nullable bool
}

// NullableOf widens a type to its nullable form (RFC-002 §6.1.3).
func NullableOf(base Type) Type {
	base.Nullable = true
	return base
}

// Value is a runtime value with its type and nilness.
type Value struct {
	Type  Type
	Data  any
	IsNil bool
}

// Widen lifts a value to its nullable type.
func Widen(value Value) Value {
	value.Type = NullableOf(value.Type)
	return value
}

// Narrow strips nullability from a value proven non-nil; without the
// proof it rejects (RFC-002 §6.2.2).
func Narrow(value Value) (Value, error) {
	if !value.Type.Nullable || value.IsNil {
		return Value{}, errors.New("nullable value lacks non-nil proof")
	}
	value.Type.Nullable = false
	return value, nil
}
