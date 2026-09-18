package anuyabi_test

import (
	"testing"

	"github.com/anuy-lang/anuy/experimental/anuyabi"
)

func TestNullableZeroValueIsNil(t *testing.T) {
	// RFC-009 §6.1.9: the Go zero value represents semantic nil without
	// breaking RFC-001 - a nullable type has a semantic nil value.
	var n anuyabi.Nullable[int]
	if !n.IsNil() {
		t.Fatal("zero value reports present")
	}
	if _, ok := n.Get(); ok {
		t.Fatal("zero value Get reports present")
	}
}

func TestSomeWrapsPresentValue(t *testing.T) {
	n := anuyabi.Some(42)
	if got, ok := n.Get(); !ok || got != 42 {
		t.Fatalf("Some(42).Get() = %v, %v", got, ok)
	}
	if n.IsNil() {
		t.Fatal("Some reports nil")
	}
}

func TestNoneIsSemanticNil(t *testing.T) {
	n := anuyabi.None[string]()
	if !n.IsNil() {
		t.Fatal("None reports present")
	}
	if _, ok := n.Get(); ok {
		t.Fatal("None Get reports present")
	}
}

func TestEveryFieldCombinationIsValid(t *testing.T) {
	// RFC-002 §6.9.6: one presence tag plus a value payload leaves no
	// invalid tag combination - every struct literal is a valid state,
	// and a present carrier may carry a zero payload.
	states := []anuyabi.Nullable[int]{
		{},
		{Present: true},
		{Value: 7},
		{Value: 7, Present: true},
	}
	for i, s := range states {
		if s.IsNil() == s.Present {
			t.Fatalf("state %d: IsNil and Present disagree", i)
		}
		if got, ok := s.Get(); ok != s.Present || (s.Present && got != s.Value) {
			t.Fatalf("state %d: Get = %v, %v", i, got, ok)
		}
	}
}

func TestStructPayloadRoundTrip(t *testing.T) {
	type point struct{ X, Y int }
	in := anuyabi.Some(point{X: 1, Y: 2})
	got, ok := in.Get()
	if !ok || got != (point{X: 1, Y: 2}) {
		t.Fatalf("struct round trip = %v, %v", got, ok)
	}
}
