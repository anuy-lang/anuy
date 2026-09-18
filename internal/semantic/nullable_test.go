package semantic

import "testing"

func TestWidenCreatesPresentNullableValue(t *testing.T) {
	integer := Type{Name: "int"}
	value := Value{Type: integer, Data: 42}
	got := Widen(value)
	if got.Type != NullableOf(integer) || got.IsNil {
		t.Fatalf("Widen() = %#v, want present int?", got)
	}
}

func TestNarrowRejectsNilNullableValue(t *testing.T) {
	integer := Type{Name: "int"}
	if _, err := Narrow(Value{Type: NullableOf(integer), IsNil: true}); err == nil {
		t.Fatal("Narrow(nil) succeeded, want proof error")
	}
}
