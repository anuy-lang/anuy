package anuyabi_test

import (
	"errors"
	"strings"
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

// boundaryPanic recovers the panic of fn and classifies it through the
// canonical carrier type (RFC-009 §6.8.12): hosts recover by type, never
// by matching message text.
func boundaryPanic(t *testing.T, fn func()) *anuyabi.InvalidForeignValue {
	t.Helper()
	var carrier *anuyabi.InvalidForeignValue
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("expected a boundary panic, got none")
			}
			err, ok := r.(error)
			if !ok {
				t.Fatalf("panic value %T does not implement error", r)
			}
			if !errors.As(err, &carrier) {
				t.Fatalf("panic value %T is not *anuyabi.InvalidForeignValue", r)
			}
		}()
		fn()
	}()
	return carrier
}

func TestRequirePanicsWithCanonicalCarrier(t *testing.T) {
	// RFC-009 §6.8.3/§6.8.12: the fail-stop boundary panic carries the
	// canonical error type with the stable message prefix, the offending
	// parameter and the violated invariant.
	carrier := boundaryPanic(t, func() {
		anuyabi.Require("user", "nil *User")
	})
	const prefix = "anuy: invalid foreign value: "
	if !strings.HasPrefix(carrier.Error(), prefix) {
		t.Fatalf("message %q misses stable prefix %q", carrier.Error(), prefix)
	}
	if want := prefix + "user: nil *User"; carrier.Error() != want {
		t.Fatalf("message = %q, want %q", carrier.Error(), want)
	}
	if carrier.Param != "user" || carrier.Reason != "nil *User" {
		t.Fatalf("fields = %q, %q", carrier.Param, carrier.Reason)
	}
}

func TestRequireEnumRange(t *testing.T) {
	// RFC-009 §6.8.6: contiguous ABI v1 - the valid discriminant range is
	// 1..variantCount; zero stays invalid (RFC-009 §6.4.3).
	anuyabi.RequireEnum("c", 1, 3)
	anuyabi.RequireEnum("c", 3, 3)
	carrier := boundaryPanic(t, func() { anuyabi.RequireEnum("c", 0, 3) })
	if carrier.Param != "c" || !strings.Contains(carrier.Reason, "discriminant") {
		t.Fatalf("zero discriminant carrier = %+v", carrier)
	}
	carrier = boundaryPanic(t, func() { anuyabi.RequireEnum("c", 4, 3) })
	if carrier.Param != "c" || !strings.Contains(carrier.Reason, "4") {
		t.Fatalf("out-of-range carrier = %+v", carrier)
	}
}

func TestRequireNonNilInterfaceRejectsUntypedNil(t *testing.T) {
	// RFC-009 §6.8.11: the untyped nil interface violates a non-null
	// native interface.
	carrier := boundaryPanic(t, func() {
		var r interface{}
		anuyabi.RequireNonNilInterface("r", r)
	})
	if carrier.Param != "r" || carrier.Reason == "" {
		t.Fatalf("untyped-nil carrier = %+v", carrier)
	}
}

// testReader and fileReader model the typed-nil hazard of RFC-007
// §6.3.7: (*fileReader)(nil) stored in the interface is non-nil at the
// interface level while the dynamic value is nil.
type testReader interface{ Read() int }

type fileReader struct{}

func (fileReader) Read() int { return 0 }

func TestRequireNonNilInterfaceRejectsTypedNil(t *testing.T) {
	// RFC-009 §6.8.11: a typed-nil dynamic value (RFC-007 §6.3.7) must
	// not enter a non-null native interface - for every reference kind.
	rejections := map[string]func(){
		"pointer":  func() { anuyabi.RequireNonNilInterface("r", (*int)(nil)) },
		"map":      func() { anuyabi.RequireNonNilInterface("r", map[string]int(nil)) },
		"chan":     func() { anuyabi.RequireNonNilInterface("r", (chan int)(nil)) },
		"func":     func() { anuyabi.RequireNonNilInterface("r", (func())(nil)) },
		"typednil": func() { var p *fileReader; anuyabi.RequireNonNilInterface("r", testReader(p)) },
	}
	for name, fn := range rejections {
		carrier := boundaryPanic(t, fn)
		if carrier.Param != "r" || carrier.Reason == "" {
			t.Fatalf("%s: carrier = %+v", name, carrier)
		}
		if !strings.Contains(carrier.Reason, "typed-nil") {
			t.Fatalf("%s: reason %q misses typed-nil wording", name, carrier.Reason)
		}
	}
}

func TestRequireNonNilInterfaceAcceptsNilSliceBacking(t *testing.T) {
	// RFC-007 §6.3.8: a nil-backed slice is a valid present value - the
	// only reference kind whose nil never violates the boundary.
	anuyabi.RequireNonNilInterface("r", []int(nil))
}

func TestRequireNonNilInterfaceAcceptsLiveValues(t *testing.T) {
	// Non-nil implementations pass silently - validation is fail-stop,
	// not a projection (RFC-007 §6.9.2: no universal failure channel).
	anuyabi.RequireNonNilInterface("r", errors.New("boom"))
	anuyabi.RequireNonNilInterface("r", []int{1})
	var p = new(int)
	anuyabi.RequireNonNilInterface("r", p)
}
