package lowering

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
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
	// tagged carrier of the anuyabi support package (RFC-009 §6.1.5–6.1.9
	// sketch), imported only when the representation is used.
	got, err := Lower("var u User?\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar u anuyabi.Nullable[User]\n\t_ = u\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerGeneratedAnuyabiImportText(t *testing.T) {
	// Pin of the emitted import line: generated Go depends on the support
	// package instead of inlining a carrier declaration.
	const want = "import \"github.com/anuy-lang/anuy/experimental/anuyabi\"\n\n"
	if anuyabiImport != want {
		t.Fatalf("anuyabi import line drifted:\n%s", anuyabiImport)
	}
}

func TestLowerNoCarrierWithoutNullable(t *testing.T) {
	// Emit-on-use: programs without tagged representations keep their
	// generated output byte-identical - no import, no declarations.
	got, err := Lower("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "anuyabi") {
		t.Fatalf("Lower() emitted the carrier dependency without nullable types: %q", got)
	}
}

func TestLowerNullableInitializerWrapsSome(t *testing.T) {
	got, err := Lower("var x int? = 42\nvar s string? = \"a\"\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"var x anuyabi.Nullable[int] = anuyabi.Some(42)", "var s anuyabi.Nullable[string] = anuyabi.Some(\"a\")"} {
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
	if !strings.Contains(got, "	u = anuyabi.None[User]()\n") {
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
	if !strings.Contains(got, "\tvar b anuyabi.Nullable[int] = a\n") {
		t.Fatalf("Lower() = %q, wants raw carrier copy", got)
	}
}

func TestLowerNullableClosureParamToGo(t *testing.T) {
	got, err := Lower("var f = func(x int?) {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tf := func(x anuyabi.Nullable[int]) {\n}\n\t_ = f\n}\n"
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
	if !strings.Contains(got, "\tvar v anuyabi.Nullable[User]\n") {
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
	if strings.Contains(got, "anuyabi") {
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
	if strings.Contains(got, "anuyabi") {
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
	for _, marker := range []string{"var s anuyabi.Nullable[[]User]", "var s2 anuyabi.Nullable[[]User]"} {
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
	if !strings.Contains(got, "\tvar xs []anuyabi.Nullable[User]\n") {
		t.Fatalf("Lower() = %q, wants nullable element slice", got)
	}
}

func TestLowerNullableNarrowingScenarioDispatch(t *testing.T) {
	// Story 13 end-to-end source (RFC-002 §6.10.2): declared `T?`, the
	// `u != nil` narrowing condition and the known-method call. The declared
	// type lowers to the carrier and the condition dispatches through the
	// support API - the verbatim struct comparison does not compile in Go
	// (mismatched types vs untyped nil). The semantic facts of the same
	// source are pinned by the integration suite (unchanged).
	got, err := Lower("var u User?\nif u != nil {\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar u anuyabi.Nullable[User]\n\tif !u.IsNil() {\n\tu.save()\n\t}\n\t_ = u\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerNativeNilConditionStaysVerbatim(t *testing.T) {
	// Native-nil shapes keep the plain Go comparison: nil is semantic nil
	// there (§6.8.1), the comparison is correct as written.
	got, err := Lower("var p *User?\nif p != nil {\np.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tif p != nil {\n") {
		t.Fatalf("Lower() = %q, wants verbatim native-nil condition", got)
	}
}

func TestLowerUntrackedConditionStaysVerbatim(t *testing.T) {
	// Without representation information the condition keeps the source
	// text - the Go semantics of an untracked operand stand as written.
	got, err := Lower("var p *User = getUser()\nif p != nil {\np.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tif p != nil {\n") {
		t.Fatalf("Lower() = %q, wants verbatim untracked condition", got)
	}
}

func TestLowerCarrierLoopConditionUsesIsNil(t *testing.T) {
	// Loop headers carry the same condition grammar (the kernel narrows
	// both forms), so the dispatch rewrite applies to `for` as well.
	got, err := Lower("var n int?\nfor n != nil {\nbreak\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tfor !n.IsNil() {\n") {
		t.Fatalf("Lower() = %q, wants IsNil dispatch in the loop header", got)
	}
}

func TestLowerSafeCallCarrierDispatch(t *testing.T) {
	// Story 13 (RFC-002 §6.4.3): `u?.m()` means the synthetic nil-guard
	// branch. The receiver is a binding - the only statement-form receiver
	// - so §6.10.3 single evaluation is structural and no synthetic
	// temporary is introduced (it enters with expression receivers).
	got, err := Lower("var u User?\nu?.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar u anuyabi.Nullable[User]\n\tif !u.IsNil() {\n\tu.Value.save()\n\t}\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeCallNativeNilGuard(t *testing.T) {
	// A native-nil receiver guards with the ordinary Go comparison and
	// calls through the value itself (nil is semantic nil, §6.8.1).
	got, err := Lower("var err error?\nerr?.Error()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar err error\n\tif err != nil {\n\terr.Error()\n\t}\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeCallUntrackedReceiverRejected(t *testing.T) {
	// Dropping the `?` would lower to a panicing deref of an absent value;
	// an untracked receiver is an explicit reject instead of verbatim.
	_, err := Lower("var u User = getUser()\nu?.save()\n")
	if err == nil || !strings.Contains(err.Error(), "safe-call receiver") {
		t.Fatalf("err = %v, want untracked safe-call receiver reject", err)
	}
}

func TestLowerSafeCallIntermediateSafeSegmentRejected(t *testing.T) {
	// Safe segments beyond the receiver have no representation model
	// (fields are outside the slice): both safe-tail chain shapes reject
	// instead of dropping the intermediate `?`.
	for _, source := range []string{
		"var u User?\nu?.a?.b()\n",
		"var u User?\nu.a?.b()\n",
	} {
		if _, err := Lower(source); err == nil || !strings.Contains(err.Error(), "safe-call chain") {
			t.Fatalf("Lower(%q) err = %v, want safe-call chain reject", source, err)
		}
	}
}

// anuyabiTypes type-checks the ABI support package from its source for the
// generated-import check (a source importer cannot resolve module paths).
type anuyabiTypes struct{}

func (anuyabiTypes) Import(path string) (*types.Package, error) {
	if path != "github.com/anuy-lang/anuy/experimental/anuyabi" {
		return nil, fmt.Errorf("unexpected import %q", path)
	}
	src, err := os.ReadFile(filepath.Join("..", "anuyabi", "anuyabi.go"))
	if err != nil {
		return nil, err
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "anuyabi.go", src, 0)
	if err != nil {
		return nil, err
	}
	// The support package imports nothing, so no nested importer is needed.
	var conf types.Config
	return conf.Check(path, fset, []*ast.File{file}, nil)
}

// typeCheckGenerated lowers source and verifies the generated Go parses and
// type-checks against the support package - the compilable end-to-end pin.
func typeCheckGenerated(t *testing.T, source string) {
	t.Helper()
	got, err := Lower(source)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "fixture.go", got, 0)
	if err != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", err, got)
	}
	conf := &types.Config{Importer: anuyabiTypes{}}
	if _, err := conf.Check("fixture", fset, []*ast.File{file}, nil); err != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", err, got)
	}
}

func TestLowerGeneratedCarrierConditionTypeChecks(t *testing.T) {
	// Compilable end-to-end pin on builtin types (story scope): `int?` plus
	// the narrowing condition - only the dispatch form compiles over the
	// carrier struct.
	typeCheckGenerated(t, "var n int? = 1\nif n != nil {\n}\n")
}

func TestLowerGeneratedNativeNilSafeCallTypeChecks(t *testing.T) {
	// The native-nil dispatch compiles end-to-end with the builtin error
	// interface as the receiver type.
	typeCheckGenerated(t, "var err error?\nerr?.Error()\n")
}

func TestLowerCarrierNilConditionUsesIsNil(t *testing.T) {
	// Story 14 (RFC-002 §6.2.3): the `x == nil` comparison over a tracked
	// carrier dispatches through the support API, mirroring the `!=` form.
	got, err := Lower("var n int?\nif n == nil {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tif n.IsNil() {\n") {
		t.Fatalf("Lower() = %q, wants IsNil dispatch for == nil", got)
	}
}

func TestLowerCarrierLoopNilConditionUsesIsNil(t *testing.T) {
	got, err := Lower("var n int?\nfor n == nil {\nbreak\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tfor n.IsNil() {\n") {
		t.Fatalf("Lower() = %q, wants IsNil dispatch in the loop header", got)
	}
}

func TestLowerNativeNilNilConditionStaysVerbatim(t *testing.T) {
	// Native-nil operands keep the plain Go comparison (nil is semantic
	// nil there, §6.8.1) - both comparison directions.
	got, err := Lower("var p *User?\nif p == nil {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tif p == nil {\n") {
		t.Fatalf("Lower() = %q, wants verbatim native-nil condition", got)
	}
}

func TestLowerUntrackedNilConditionStaysVerbatim(t *testing.T) {
	got, err := Lower("var p *User = getUser()\nif p == nil {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tif p == nil {\n") {
		t.Fatalf("Lower() = %q, wants verbatim untracked condition", got)
	}
}

func TestLowerSafeValueCarrierReceiverCarrierTarget(t *testing.T) {
	// Story 14 (RFC-002 §6.4.5, §6.10.2): a safe-tail value with a declared
	// nullable target lowers to the synthetic guard branch - Some(member)
	// in the non-nil branch, None in the nil branch.
	got, err := Lower("var a User?\nvar c int? = a?.count\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar a anuyabi.Nullable[User]\n\tvar c anuyabi.Nullable[int]\n\tif !a.IsNil() {\n\tc = anuyabi.Some(a.Value.count)\n\t} else {\n\tc = anuyabi.None[int]()\n\t}\n\t_ = c\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeValueNativeNilReceiverCarrierTarget(t *testing.T) {
	// A native-nil receiver guards with the plain Go comparison and reads
	// the member off the value itself.
	got, err := Lower("var err error?\nvar s string? = err?.Error()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar err error\n\tvar s anuyabi.Nullable[string]\n\tif err != nil {\n\ts = anuyabi.Some(err.Error())\n\t} else {\n\ts = anuyabi.None[string]()\n\t}\n\t_ = s\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeValueCarrierReceiverNativeNilTarget(t *testing.T) {
	// A native-nil target takes the raw member in the non-nil branch and
	// Go nil as semantic nil in the nil branch (§6.8.1).
	got, err := Lower("var a User?\nvar p *User? = a?.profile\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar a anuyabi.Nullable[User]\n\tvar p *User\n\tif !a.IsNil() {\n\tp = a.Value.profile\n\t} else {\n\tp = nil\n\t}\n\t_ = p\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeValueAssignCarrierTarget(t *testing.T) {
	// The assignment form dispatches through the tracked target binding.
	got, err := Lower("var a User?\nvar c int?\nc = a?.count\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar a anuyabi.Nullable[User]\n\tvar c anuyabi.Nullable[int]\n\tif !a.IsNil() {\n\tc = anuyabi.Some(a.Value.count)\n\t} else {\n\tc = anuyabi.None[int]()\n\t}\n\t_ = c\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeValueCallMemberDispatch(t *testing.T) {
	// A safe method value dispatches identically; the call evaluates only
	// in the non-nil branch (§6.10.4 analogue). The receiver is a binding,
	// so §6.10.3 single evaluation stays structural - no temporary.
	got, err := Lower("var n int?\nvar b bool? = n?.IsNil()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar n anuyabi.Nullable[int]\n\tvar b anuyabi.Nullable[bool]\n\tif !n.IsNil() {\n\tb = anuyabi.Some(n.Value.IsNil())\n\t} else {\n\tb = anuyabi.None[bool]()\n\t}\n\t_ = b\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSafeValueRejectsUndeclaredTarget(t *testing.T) {
	// Without a declared nullable target the None[elem] branch cannot be
	// spelled: inferred and non-null-typed targets reject.
	for _, source := range []string{
		"var a User?\nvar c = a?.count\n",
		"var a User?\nvar c int = a?.count\n",
		"var a User?\nvar c = 1\nc = a?.count\n",
	} {
		if _, err := Lower(source); err == nil || !strings.Contains(err.Error(), "not a declared nullable") {
			t.Fatalf("Lower(%q) err = %v, want declared nullable target reject", source, err)
		}
	}
}

func TestLowerSafeValueRejectsUntrackedReceiver(t *testing.T) {
	// An untracked binding and a root-call receiver (its call is invisible
	// in NavigationExpr) carry no representation information.
	for _, source := range []string{
		"var a User = getUser()\nvar c int? = a?.count\n",
		"var c int? = findUser()?.id\n",
	} {
		if _, err := Lower(source); err == nil || !strings.Contains(err.Error(), "not a tracked nullable") {
			t.Fatalf("Lower(%q) err = %v, want untracked receiver reject", source, err)
		}
	}
}

func TestLowerSafeValueRejectsChainsBeyondReceiver(t *testing.T) {
	// Safe segments beyond the receiver have no representation model
	// (mixed chains, multi-safe chains) - reject, not verbatim.
	for _, source := range []string{
		"var a User?\nvar c int? = a.addr?.code\n",
		"var a User?\nvar c int? = a?.x?.y\n",
	} {
		if _, err := Lower(source); err == nil || !strings.Contains(err.Error(), "beyond the receiver") {
			t.Fatalf("Lower(%q) err = %v, want chain reject", source, err)
		}
	}
}

func TestLowerSafeValueRejectsUnpairedTuple(t *testing.T) {
	// The single-value multi-target form has no pairing, so no tracked
	// target exists for the dispatch.
	if _, err := Lower("var c, d = 1, 2\nc, d = a?.b\n"); err == nil || !strings.Contains(err.Error(), "unpaired") {
		t.Fatalf("err = %v, want unpaired safe-value reject", err)
	}
}

func TestLowerRejectsSafeTailRangeOperand(t *testing.T) {
	// Iteration over a nullable collection has no defined semantics yet
	// (RFC-003 iteration slice): the safe-tail range operand rejects
	// instead of emitting range u?.items.
	if _, err := Lower("var u User?\nfor x in u?.items {\n}\n"); err == nil || !strings.Contains(err.Error(), "safe-tail range operand") {
		t.Fatalf("err = %v, want safe-tail range operand reject", err)
	}
}

func TestLowerOrdinaryNavigationStaysVerbatim(t *testing.T) {
	// Ordinary (non-safe) navigation keeps the verbatim lowering in every
	// target shape - the dispatch touches only safe tails.
	for _, source := range []string{
		"var c = a.b\n",
		"var c int = a.b\n",
		"var a User\nvar c int = a.count\n",
	} {
		if _, err := Lower(source); err != nil {
			t.Fatalf("Lower(%q) err = %v, want verbatim ordinary navigation", source, err)
		}
	}
}

func TestLowerGeneratedNativeNilReceiverSafeValueTypeChecks(t *testing.T) {
	// Compilable end-to-end for the native-nil receiver form. The carrier-
	// receiver form has no builtin shape to type-check (builtin value types
	// have no members - Nullable.IsNil is the carrier API, not an Anuy
	// member of T) and stays pinned by exact text.
	typeCheckGenerated(t, "var err error?\nvar s string? = err?.Error()\n")
}

func TestLowerGeneratedCarrierNilConditionTypeChecks(t *testing.T) {
	// The == nil dispatch form compiles where the verbatim struct
	// comparison does not (mirror of the story 13 != pin).
	typeCheckGenerated(t, "var n int? = 1\nif n == nil {\n}\n")
}

func TestLowerSafeValueRejectsCallArguments(t *testing.T) {
	// Mid-chain call arguments are invisible in NavigationExpr; a call
	// with arguments cannot dispatch (only the zero-argument member call
	// is part of the slice).
	if _, err := Lower("var a User?\nvar c int? = a?.sum(1)\n"); err == nil || !strings.Contains(err.Error(), "call arguments") {
		t.Fatalf("err = %v, want safe-value call arguments reject", err)
	}
}

func TestLowerCallStatementArguments(t *testing.T) {
	// Story 15 (the story 07 pre-existing gap): call statement arguments
	// emit into the call - today they are dropped entirely.
	got, err := Lower("var u User\nu.m(a, b)\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tu.m(a, b)\n") {
		t.Fatalf("Lower() = %q, wants emitted arguments", got)
	}
}

func TestLowerCallStatementClosureArgument(t *testing.T) {
	// A closure argument renders as the Go func literal (RFC-003 §41
	// multiline form parses into the same Closure value).
	got, err := Lower("var u User\nu.m(func(x int) {\ny = x\n})\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tu.m(func(x int) {\n\ty = x\n})\n") {
		t.Fatalf("Lower() = %q, wants the closure literal argument", got)
	}
}

func TestLowerSafeCallArgumentsInsideGuard(t *testing.T) {
	// RFC-002 §6.10.4: the argument of a safe call evaluates only in the
	// non-nil branch - placement inside the guard is the pin.
	got, err := Lower("var u User?\nu?.send(payload())\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func Run() {\n\tvar u anuyabi.Nullable[User]\n\tif !u.IsNil() {\n\tu.Value.send(payload())\n\t}\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerRejectsSafeTailCallArgument(t *testing.T) {
	// A safe-tail argument has no dispatch target in argument position
	// (the story 14 argument) - reject in both call forms.
	for _, source := range []string{
		"var u User\nu.m(a?.b)\n",
		"var u User?\nu?.m(a?.b)\n",
	} {
		if _, err := Lower(source); err == nil || !strings.Contains(err.Error(), "safe-tail call argument") {
			t.Fatalf("Lower(%q) err = %v, want safe-tail call argument reject", source, err)
		}
	}
}

func TestLowerGeneratedCallArgumentsTypeChecks(t *testing.T) {
	// Compilable end-to-end: a predeclared callee is the one builtin shape
	// with arguments representable on builtins.
	typeCheckGenerated(t, "println(\"hello\")\n")
}

func TestLowerFuncDeclVoidToGo(t *testing.T) {
	// Story 16: top-level functions hoist to package-level declarations
	// before Run; a bare return stays verbatim.
	got, err := Lower("func f() {\nreturn\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc f() {\n\treturn\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerFuncDeclResultAndCallE2E(t *testing.T) {
	// A free function with a non-null result plus its call statement.
	got, err := Lower("func twice(n int) int {\nreturn n + n\n}\ntwice(1)\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc twice(n int) int {\n\treturn n + n\n}\n\nfunc Run() {\n\ttwice(1)\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerFuncDeclCarrierResultNone(t *testing.T) {
	// `return nil` over a declared `T?` result converts to the None carrier
	// (§6.7-6.8, the convert rules apply in return position).
	got, err := Lower("func find() User? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "func find() anuyabi.Nullable[User] {\n\treturn anuyabi.None[User]()\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerFuncDeclReturnSomeAndCarrierCopy(t *testing.T) {
	// The Run body lowers before the hoisting pass, so the function body
	// sees the tracked bindings: a carrier copy returns passthrough, other
	// values wrap in Some.
	got, err := Lower("var u User?\nfunc find() User? {\nreturn u\n}\nfunc pick() User? {\nreturn 42\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func find() anuyabi.Nullable[User] {\n\treturn u\n}") {
		t.Fatalf("Lower() = %q, wants carrier copy passthrough", got)
	}
	if !strings.Contains(got, "func pick() anuyabi.Nullable[User] {\n\treturn anuyabi.Some(42)\n}") {
		t.Fatalf("Lower() = %q, wants Some wrapping", got)
	}
}

func TestLowerFuncDeclNativeNilResult(t *testing.T) {
	// Native-nil results keep the plain Go type; nil returns stay verbatim.
	got, err := Lower("func wrap() error? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc wrap() error {\n\treturn nil\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerFuncDeclCarrierParam(t *testing.T) {
	// Parameters lower through the representation rules (§6.7-6.8).
	got, err := Lower("func show(n int?) {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func show(n anuyabi.Nullable[int]) {\n}") {
		t.Fatalf("Lower() = %q, wants carrier parameter", got)
	}
}

func TestLowerMethodDeclToGo(t *testing.T) {
	// A method emits as a Go method with the synthetic receiver (the
	// grammar has no receiver binding - the body never reads it).
	got, err := Lower("func User.save() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func (anuyRecv User) save() {\n}") {
		t.Fatalf("Lower() = %q, wants the synthetic receiver method", got)
	}
}

func TestLowerMethodDeclCarrierResult(t *testing.T) {
	got, err := Lower("func User.find() User? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func (anuyRecv User) find() anuyabi.Nullable[User] {\n\treturn anuyabi.None[User]()\n}") {
		t.Fatalf("Lower() = %q, wants the method with carrier result", got)
	}
}

func TestLowerNestedFuncDeclRejected(t *testing.T) {
	// A block-nested declaration has no Go shape (kernel accepts it - a
	// narrowing reject, not a silent drop).
	if _, err := Lower("{\nfunc f() int {\nreturn 1\n}\n}\n"); err == nil || !strings.Contains(err.Error(), "nested function") {
		t.Fatalf("err = %v, want nested function reject", err)
	}
}

func TestLowerGeneratedFuncCallsTypeCheck(t *testing.T) {
	typeCheckGenerated(t, "func twice(n int) int {\nreturn n + n\n}\ntwice(1)\n")
}

func TestLowerGeneratedFuncCarrierResultTypeChecks(t *testing.T) {
	typeCheckGenerated(t, "func find() int? {\nreturn nil\n}\n")
}

func TestLowerGeneratedFuncCarrierParamTypeChecks(t *testing.T) {
	typeCheckGenerated(t, "func show(n int?) {\n}\n")
}

func TestLowerStructDeclarationToGo(t *testing.T) {
	// Story 21 (RFC-014 6.2, 6.13): a struct declaration hoists to an
	// ordinary Go struct, field order preserved, field types through the
	// representation rules (`*User?` keeps the plain Go pointer).
	got, err := Lower("type User struct {\nid UserID\nname string\nmanager *User?\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\ntype User struct {\n\tid UserID\n\tname string\n\tmanager *User\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerStructFieldTagged(t *testing.T) {
	// A nullable field of a value type takes the carrier (6.8.16).
	got, err := Lower("type Holder struct {\nn int?\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n" + anuyabiImport + "type Holder struct {\n\tn anuyabi.Nullable[int]\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerEnumDeclarationToGo(t *testing.T) {
	// Story 30 (RFC-009 §6.4): a named uint32 with discriminants 1..N in
	// declaration order - the Go zero value stays the reserved invalid
	// representation (§6.4.2-6.4.3); constant names per §6.4.5.
	got, err := Lower("type Color enum {\nRed\nGreen\nBlue\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\ntype Color uint32\n\nconst (\n\tColorRed Color = 1\n\tColorGreen Color = 2\n\tColorBlue Color = 3\n)\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerSwitchToGo(t *testing.T) {
	// Story 31 (RFC-009 §6.4, §6.4.5): the switch lowers to an ordinary
	// Go switch; case labels are the §6.4.5 constant names.
	got, err := Lower("type Color enum {\nRed\nGreen\n}\nvar c = Color.Red\nswitch c {\ncase Color.Red:\nvar x = 1\ncase Color.Green:\nvar x = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tswitch c {\n\tcase ColorRed:\n") || !strings.Contains(got, "\tcase ColorGreen:\n") {
		t.Fatalf("Lower() = %q, wants the Go switch with variant case labels", got)
	}
}

func TestLowerValueSwitchToGo(t *testing.T) {
	// Story 32 (RFC-006 §6.4.4, RFC-009 §6.4): the value switch lowers to
	// the declared result plus an ordinary Go switch assigning the arm
	// values - Go has no switch expressions.
	got, err := Lower("type Color enum {\nRed\nGreen\n}\nvar c = Color.Red\nvar text string = switch c {\ncase Color.Red:\n\"red\"\ncase Color.Green:\n\"green\"\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tvar text string\n") || !strings.Contains(got, "\tswitch c {\n\tcase ColorRed:\n\t\ttext = \"red\"\n") {
		t.Fatalf("Lower() = %q, wants the declared result plus switch assignments", got)
	}
}

func TestLowerValueSwitchInferredRejected(t *testing.T) {
	// §6.4.3 inference needs type identity the layer does not track - the
	// §6.4.4 declared form is the v1 lowering.
	if _, err := Lower("type Color enum {\nRed\n}\nvar c = Color.Red\nvar text = switch c {\ncase Color.Red:\n\"red\"\n}\n"); err == nil || !strings.Contains(err.Error(), "declared result type") {
		t.Fatalf("err = %v, want the declared-result-type reject", err)
	}
}

func TestLowerNullableEnumSwitchToGo(t *testing.T) {
	// Story 33 (RFC-006 §6.5, §6.11.3): a nullable-enum scrutinee lowers
	// to the representation-specific nullable test plus the Value
	// dispatch; the synthetic default guards the foreign boundary
	// (RFC-009 §6.4.3).
	got, err := Lower("type Color enum {\nRed\nGreen\n}\nvar c Color? = nil\nswitch c {\ncase nil:\nvar x = 1\ncase Color.Red:\nvar y = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\tswitch {\n",
		"\tcase c.IsNil():\n",
		"\tcase c.Value == ColorRed:\n",
		"panic(\"anuy: invalid enum discriminant\")",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Lower() = %q, wants %q", got, want)
		}
	}
}

func TestLowerReturnErrorAndTry(t *testing.T) {
	// Story 34 (RFC-005 §6.4.2/§6.5.3, RFC-009 §6.6.1/§6.6.6/§6.6.8): the
	// failure return lowers to the plain error return (§6.6.1 - error-only,
	// no padding); error-only try lowers to the exactly-once guard.
	got, err := Lower("func Save() error? {\nreturn error nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func Save() error {\n\treturn nil\n") {
		t.Fatalf("Lower() = %q, wants the plain error return", got)
	}
	tryGot, err := Lower("func Log(msg string) error? {\nreturn nil\n}\nfunc Save() error? {\ntry Log(\"x\")\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tryGot, "\tif err := Log(\"x\"); err != nil {\n\t\treturn err\n\t}\n") {
		t.Fatalf("tryGot = %q, wants the propagation guard", tryGot)
	}
}

func TestLowerNestedTypeDeclRejected(t *testing.T) {
	// A block-nested declaration has no Go shape (kernel accepts it - a
	// narrowing reject).
	if _, err := Lower("{\ntype User struct {\n}\n}\n"); err == nil || !strings.Contains(err.Error(), "nested type declaration") {
		t.Fatalf("err = %v, want nested type declaration reject", err)
	}
}

func TestLowerGeneratedStructConstructionTypeChecks(t *testing.T) {
	// The story-10 blocker closes end-to-end: a user type is declared,
	// constructed and passed to a declared function.
	typeCheckGenerated(t, "type User struct {\nid int\n}\nfunc save(u User) {\n}\nvar u = User{id: 1}\nsave(u)\n")
}

func TestLowerFieldMutationToGo(t *testing.T) {
	// Story 22 (RFC-014 6.7, 6.13): field mutation lowers verbatim - the
	// type-free layer performs no field-type conversions.
	got, err := Lower("type User struct {\nname string\n}\nvar u = User{name: \"Ann\"}\nu.name = \"Bob\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tu.name = \"Bob\"\n") {
		t.Fatalf("Lower() = %q, wants the field mutation", got)
	}
}

func TestLowerMultilineConstructionVerbatim(t *testing.T) {
	// Story 24 (RFC-014 6.4, 6.5): the multiline construction lowers
	// verbatim - the source order of the field initializers is the
	// generated order (§6.5 example shape).
	got, err := Lower("type Pair struct {\nright int\nleft int\n}\nvar pair = Pair{\nright: nextRight(),\nleft: nextLeft(),\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tpair := Pair{\nright: nextRight(),\nleft: nextLeft(),\n}\n") {
		t.Fatalf("Lower() = %q, wants the verbatim multiline construction", got)
	}
}

func TestLowerGeneratedMultilineConstructionTypeChecks(t *testing.T) {
	// The §6.4 trailing-comma MUST is what keeps the verbatim multiline
	// literal valid Go: the generated file parses and type-checks.
	typeCheckGenerated(t, "type User struct {\nid int\nname string\n}\nvar u = User{\nid: 1,\nname: \"Ann\",\n}\n")
}

func TestLowerStrictFallibleFunctions(t *testing.T) {
	// Story 35 (RFC-005 §6.2.3/§6.4.1/§6.4.2, RFC-009 §6.6.2–6.6.6): the
	// strict fallible Go ABI `(Data, error)`, the failure-return padding
	// slots (§6.6.3–6.6.5), and the value-try temporaries with the
	// immediate propagation branch (§6.6.6).
	got, err := Lower("type Data struct {\nid int\n}\nfunc LoadData(path string) (Data, error?) {\nreturn Data{id: 1}, nil\n}\nfunc Load(path string) (Data, error?) {\nif path == \"\" {\nreturn error errors.New(\"empty path\")\n}\nvar d = try LoadData(path)\nreturn d, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"func LoadData(path string) (Data, error) {",
		"func Load(path string) (Data, error) {",
		"\tvar __anuy_pad0 Data\n\treturn __anuy_pad0, errors.New(\"empty path\")\n",
		"\t__anuy_v2, __anuy_err1 := LoadData(path)\n\tif __anuy_err1 != nil {\n\t\tvar __anuy_pad3 Data\n\t\treturn __anuy_pad3, __anuy_err1\n\t}\n\td := __anuy_v2\n",
		"\treturn d, nil\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Lower() = %q, wants %q", got, want)
		}
	}
}

func TestLowerGeneratedStrictFallibleTypeChecks(t *testing.T) {
	// Story 35: the strict fallible shape compiles end-to-end - the Go
	// ABI, the padding slots and the value-try temporaries type-check
	// against the declared struct. The failure operand comes from a
	// declared non-null parameter (the errors package is not imported by
	// the minimal generator).
	typeCheckGenerated(t, "type Data struct {\nid int\n}\nfunc LoadData(path string) (Data, error?) {\nreturn Data{id: 1}, nil\n}\nfunc Load(path string, cause error) (Data, error?) {\nif path == \"\" {\nreturn error cause\n}\nvar d = try LoadData(path)\nreturn d, nil\n}\n")
}

func TestLowerValueTryErrorOnlyEnclosing(t *testing.T) {
	// Story 35 (RFC-009 §6.6.6): a value-try inside an error-only
	// function propagates the bare error - no padding slots.
	got, err := Lower("func Load() (Data, error?) {\nreturn Data{}, nil\n}\nfunc Save() error? {\nvar d = try Load()\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\t__anuy_v1, __anuy_err0 := Load()\n\tif __anuy_err0 != nil {\n\t\treturn __anuy_err0\n\t}\n\td := __anuy_v1\n") {
		t.Fatalf("Lower() = %q, wants the error-only propagation", got)
	}
}

func TestLowerFallibleDestructuring(t *testing.T) {
	// Story 36 (RFC-005 §6.3, RFC-009 §6.6.9): the flat destructuring
	// lowers to the ordinary Go comma-ok assignment; the guard-join stays
	// verbatim (the failure branch carries the §6.6.3 padding).
	got, err := Lower("type Data struct {\nid int\n}\nfunc LoadData(path string) (Data, error?) {\nreturn Data{id: 1}, nil\n}\nfunc Load(path string) (Data, error?) {\nvar data, err = LoadData(path)\nif err != nil {\nreturn error err\n}\nreturn data, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"\tdata, err := LoadData(path)\n",
		"\tvar __anuy_pad0 Data\n\treturn __anuy_pad0, err\n",
		"\treturn data, nil\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Lower() = %q, wants %q", got, want)
		}
	}
}

func TestLowerDiscardStatement(t *testing.T) {
	// Story 37 (RFC-005 §6.6.2): the discarded call lowers verbatim - Go
	// discards return values naturally, the flag is a source-level intent.
	got, err := Lower("func Log(msg string) error? {\nreturn nil\n}\nfunc Save() error? {\ndiscard Log(\"done\")\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "\tLog(\"done\")\n") {
		t.Fatalf("Lower() = %q, wants the verbatim call", got)
	}
}

func TestLowerInterfaceDeclAndImpl(t *testing.T) {
	// Story 39 (RFC-004 §6.8.1–6.8.3, RFC-009 §6.5.1–6.5.4): the
	// interface lowers to an ordinary Go interface; impl erases; the
	// pointer receiver lowers to the Go pointer form.
	got, err := Lower("type Data struct {\nid int\n}\ninterface Reader {\nRead(d Data) Data\n}\ntype File struct {\nid int\n}\nfunc *File.Read(d Data) Data {\nreturn d\n}\nimpl Reader for *File\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"type Reader interface {\n\tRead(d Data) Data\n}\n",
		"func (anuyRecv *File) Read(d Data) Data {",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Lower() = %q, wants %q", got, want)
		}
	}
	if strings.Contains(got, "impl") {
		t.Fatalf("Lower() = %q, wants impl erased (§6.8.2)", got)
	}
}

func TestLowerGeneratedInterfaceTypeChecks(t *testing.T) {
	// Story 39: the interface, its Go method set and the conformance
	// conversion type-check end-to-end (value receiver - the Go method
	// set of File carries Read).
	typeCheckGenerated(t, "type Data struct {\nid int\n}\ninterface Reader {\nRead(d Data) Data\n}\ntype File struct {\nid int\n}\nfunc File.Read(d Data) Data {\nreturn d\n}\nimpl Reader for File\nvar r Reader = File{id: 1}\nvar x = r\n")
}
