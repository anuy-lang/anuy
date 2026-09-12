package semantic

import "errors"

type Type struct {
	Name     string
	Nullable bool
}

func NullableOf(base Type) Type {
	base.Nullable = true
	return base
}

type Value struct {
	Type  Type
	Data  any
	IsNil bool
}

func Widen(value Value) Value {
	value.Type = NullableOf(value.Type)
	return value
}

func Narrow(value Value) (Value, error) {
	if !value.Type.Nullable || value.IsNil {
		return Value{}, errors.New("nullable value lacks non-nil proof")
	}
	value.Type.Nullable = false
	return value, nil
}
