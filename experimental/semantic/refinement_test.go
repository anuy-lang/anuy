package semantic

import "testing"

func TestRefinementIsTrackedPerBinding(t *testing.T) {
	r := NewRefinements()
	r.MarkNonNil(1)
	if !r.IsNonNil(1) || r.IsNonNil(2) {
		t.Fatal("refinement must belong only to checked BindingID")
	}
}

func TestSafeNavigateEvaluatesReceiverOnceAndSkipsArgumentsOnNil(t *testing.T) {
	receiverCalls, argumentCalls := 0, 0
	got := SafeNavigate(func() Value { receiverCalls++; return Value{Type: NullableOf(Type{Name: "User"}), IsNil: true} }, func(Value) Value { argumentCalls++; return Value{} })
	if receiverCalls != 1 || argumentCalls != 0 || !got.IsNil {
		t.Fatalf("receiver=%d arguments=%d result=%#v", receiverCalls, argumentCalls, got)
	}
}
