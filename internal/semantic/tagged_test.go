package semantic

import "testing"

func TestTaggedNullableRoundTripsPresentAndNil(t *testing.T) {
	present := Present(42)
	if got, ok := present.Get(); !ok || got != 42 {
		t.Fatalf("present.Get() = %v, %v", got, ok)
	}
	nilValue := Nil[int]()
	if _, ok := nilValue.Get(); ok {
		t.Fatal("Nil.Get() reported present")
	}
}

func TestTaggedNullableDoesNotUsePointerRepresentation(t *testing.T) {
	value := Present("x")
	if value.Present != true || value.Value != "x" {
		t.Fatalf("tagged value = %#v", value)
	}
}
