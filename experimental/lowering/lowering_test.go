package lowering

import (
	"strings"
	"testing"
)

func TestLowerVarAssignmentToGo(t *testing.T) {
	got, err := Lower("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int\n\tx = 1\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerMultipleAssignmentToGo(t *testing.T) {
	got, err := Lower("var x int = 1\nvar y int = 2\nx, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\tvar y int = 2\n\tx, y = y, x\n\t_ = y\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNullableVarDeclToGo(t *testing.T) {
	// Story 10, PF-G-06 flip (ADR-0003): a declared `T?` lowers to the
	// tagged carrier; the prelude appears because the representation is
	// used (RFC-002 §6.7–6.8).
	got, err := Lower("var u User?\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + nullablePrelude + "func Run() {\n\tvar u Nullable[User]\n\t_ = u\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNullablePreludeText(t *testing.T) {
	// Pin of the emitted carrier text: struct layout and the §6.9.5 helper
	// sketch (Some/None/Get/IsNil). The zero value is semantic nil
	// (RFC-009 §6.1.9 sketch); the final public API stays with RFC-009.
	const want = `// Nullable is the experimental tagged representation of a nullable value
// type (RFC-002 §6.7–6.8): Present == false is semantic nil.
type Nullable[T any] struct {
	Value   T
	Present bool
}

func Some[T any](value T) Nullable[T] {
	return Nullable[T]{Value: value, Present: true}
}

func None[T any]() Nullable[T] {
	return Nullable[T]{}
}

func (n Nullable[T]) Get() (T, bool) {
	return n.Value, n.Present
}

func (n Nullable[T]) IsNil() bool {
	return !n.Present
}

`
	if nullablePrelude != want {
		t.Fatalf("nullable prelude drifted:\n%s", nullablePrelude)
	}
}

func TestLowerNoPreludeWithoutNullable(t *testing.T) {
	// Emit-on-use: programs without tagged representations keep their
	// generated output byte-identical (no carrier prelude).
	got, err := Lower("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "Nullable") {
		t.Fatalf("Lower() emitted the carrier prelude without nullable types: %q", got)
	}
}

func TestLowerNullableInitializerWrapsSome(t *testing.T) {
	got, err := Lower("var x int? = 42\nvar s string? = \"a\"\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"var x Nullable[int] = Some(42)", "var s Nullable[string] = Some(\"a\")"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("Lower() = %q, misses %q", got, marker)
		}
	}
}

func TestLowerAssignNilToNullableUsesNone(t *testing.T) {
	got, err := Lower("var u User?\nu = nil\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tu = None[User]()\n") {
		t.Fatalf("Lower() = %q, wants None conversion", got)
	}
}

func TestLowerNullableCopyPassesThrough(t *testing.T) {
	// A copy of an equally-tagged binding already holds the carrier
	// representation; wrapping it in Some would fake presence.
	got, err := Lower("var a int? = 1\nvar b int? = a\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar b Nullable[int] = a\n") {
		t.Fatalf("Lower() = %q, wants raw carrier copy", got)
	}
}

func TestLowerNullableClosureParamToGo(t *testing.T) {
	got, err := Lower("var f = func(x int?) {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + nullablePrelude + "func Run() {\n\tf := func(x Nullable[int]) {\n}\n\t_ = f\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNullableIdempotence(t *testing.T) {
	// `(T?)? = T?` (§6.1.12): the carrier wrapper is never nested.
	got, err := Lower("var v (User?)?\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar v Nullable[User]\n") {
		t.Fatalf("Lower() = %q, wants single carrier", got)
	}
}

func TestLowerClosureLiteralToGo(t *testing.T) {
	got, err := Lower("var x int = 1\nvar increment = func() {\nx = x + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\tincrement := func() {\n\tx = x + 1\n}\n\t_ = increment\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerConditionLoopToGo(t *testing.T) {
	got, err := Lower("var ready bool = true\nvar x int\nfor ready {\nx = 1\n}\nx = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar ready bool = true\n\tvar x int\n\tfor ready {\n\tx = 1\n\t}\n\tx = 2\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerInfiniteLoopToGo(t *testing.T) {
	got, err := Lower("var x int\nfor {\nif x == 1 {\nbreak\n}\n}\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int\n\tfor {\n\tif x == 1 {\n\tbreak\n\t}\n\t}\n\tx = 1\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerIterationLoopToGo(t *testing.T) {
	got, err := Lower("var total int\nfor user in users {\ntotal = total + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar total int\n\tfor _, user := range users {\n\ttotal = total + 1\n\t}\n\t_ = total\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerBreakContinuePassthrough(t *testing.T) {
	got, err := Lower("var i int = 0\nfor {\ni = i + 1\nif i == 3 {\nbreak\n}\nif i == 2 {\ncontinue\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar i int = 0\n\tfor {\n\ti = i + 1\n\tif i == 3 {\n\tbreak\n\t}\n\tif i == 2 {\n\tcontinue\n\t}\n\t}\n\t_ = i\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerRejectsUnsupportedRangeOperand(t *testing.T) {
	_, err := Lower("for x in getItems() {\n}\n")
	if err == nil {
		t.Fatal("unsupported range operand lowered")
	}
	if !strings.Contains(err.Error(), "unsupported range operand") {
		t.Fatalf("err = %v, want unsupported range operand error", err)
	}
}

func TestLowerBlockToGo(t *testing.T) {
	got, err := Lower("var x int = 1\n{\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\t{\n\tx = 2\n\t}\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerBlankDiscardToGo(t *testing.T) {
	got, err := Lower("var value, _ = f()\n_, y = 1, 2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvalue, _ := f()\n\t_, y = 1, 2\n\t_ = y\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerCallStatementsToGo(t *testing.T) {
	// Story 05: call statements are the effectful statement form; the call
	// value is discarded by statement semantics, so no blank discard line
	// follows and none feeds the trailing `_ =`.
	got, err := Lower("var user User = getUser()\nclear()\nuser.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar user User = getUser()\n\tclear()\n\tuser.save()\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNullablePointerStaysNative(t *testing.T) {
	// §6.8.1/§6.8.4 (ADR-0003): valid `*User` excludes nil, so `*User?`
	// reuses the plain Go pointer - nil is semantic nil, no carrier.
	got, err := Lower("var p *User?\np = nil\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar p *User\n\tp = nil\n\t_ = p\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNullableMapStaysNative(t *testing.T) {
	// §6.8.6: `(map[string]User)?` keeps the ordinary Go map; the `?` on a
	// map type is whole-map nullability (canonical spelling, PF-A-12).
	got, err := Lower("var m map[string]User?\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar m map[string]User\n") {
		t.Fatalf("Lower() = %q, wants native map", got)
	}
	if strings.Contains(got, "Nullable") {
		t.Fatalf("Lower() emitted the carrier for a native-nil shape: %q", got)
	}
}

func TestLowerNullableErrorStaysNative(t *testing.T) {
	// `error?` uses the native nil interface (RFC-002 §6.9.2, RFC-009
	// §6.2.8 sketch): Go nil error is absence.
	got, err := Lower("var err error?\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar err error\n") {
		t.Fatalf("Lower() = %q, wants native error", got)
	}
	if strings.Contains(got, "Nullable") {
		t.Fatalf("Lower() emitted the carrier for error?: %q", got)
	}
}

func TestLowerPlainSliceStaysNilBacked(t *testing.T) {
	// §6.8.9: a plain `[]T` keeps the ordinary Go slice, nil backing
	// included as a valid present value.
	got, err := Lower("var xs []User\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar xs []User\n") {
		t.Fatalf("Lower() = %q, wants plain slice", got)
	}
}

func TestLowerNullableSliceUsesCarrier(t *testing.T) {
	// §6.8.10–6.8.13: nil is already a valid `[]T`, so `[]T?` must tag:
	// `Nullable[[]User]` distinguishes semantic nil from a present
	// nil-backed slice.
	got, err := Lower("var s []User?\nvar s2 ([]User)?\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"var s Nullable[[]User]", "var s2 Nullable[[]User]"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("Lower() = %q, misses %q", got, marker)
		}
	}
}

func TestLowerSliceOfNullableElements(t *testing.T) {
	// `[](User?)`: nullability sits on the elements, the slice stays plain.
	got, err := Lower("var xs [](User?)\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar xs []Nullable[User]\n") {
		t.Fatalf("Lower() = %q, wants nullable element slice", got)
	}
}

func TestLowerNullableNarrowingScenarioVerbatim(t *testing.T) {
	// Task 5 end-to-end source: declared `T?`, the `u != nil` narrowing
	// condition and the known-method call. The declared type lowers to the
	// carrier; conditions and calls pass through verbatim - the branch
	// dispatch over the carrier is a separate slice. The semantic facts of
	// the same source are pinned by the integration suite (unchanged).
	got, err := Lower("var u User?\nif u != nil {\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + nullablePrelude + "func Run() {\n\tvar u Nullable[User]\n\tif u != nil {\n\tu.save()\n\t}\n\t_ = u\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}
