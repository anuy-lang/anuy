package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 71 (RFC-019 §6.14): variadicness is part of function type
// identity, and the kernel's per-argument checks fan out over the
// variadic element class.

func TestVariadicIdentityMismatchReports(t *testing.T) {
	// §6.14: `func(...int)` and `func(int) int` are distinct types - a
	// non-variadic closure does not satisfy a variadic target.
	result, err := AnalyzeSource("var cb func(...int) int = func(v int) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
	got := result.Diagnostics[0]
	if got.Code != "ANUY7008" || got.Severity != semantic.SeverityError {
		t.Fatalf("FunctionTypeMismatch = (%s, %s), want (ANUY7008, Error)", got.Code, got.Severity)
	}
}

func TestVariadicIdentityMatchClean(t *testing.T) {
	result, err := AnalyzeSource("var cb func(...int) int = func(v ...int) int {\nreturn 0\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestVariadicSliceSpellingMismatchReports(t *testing.T) {
	// §6.14: `func(...int)` and `func([]int)` are distinct types.
	result, err := AnalyzeSource("var cb func(...int) int = func(v []int) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
}

func TestVariadicElementArgumentD3FanOut(t *testing.T) {
	// Individual variadic arguments carry the element class: a nullable
	// argument beyond the fixed prefix reports D-3 like a fixed-position
	// one.
	result, err := AnalyzeSource("func Sum(total int, values ...int) int {\nreturn total\n}\nvar u User? = findUser()\nSum(1, 2, u)\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableArgument")
	got := result.Diagnostics[0]
	if got.Code != "ANUY4003" || got.Severity != semantic.SeverityError {
		t.Fatalf("NullableArgument = (%s, %s), want (ANUY4003, Error)", got.Code, got.Severity)
	}
}

func TestVariadicSpreadArgumentClean(t *testing.T) {
	// A spread argument is the slice value: its element nullability is the
	// go-stage's domain (story 64), the kernel does not report D-3 for it.
	result, err := AnalyzeSource("func Sum(values ...int) int {\nreturn 0\n}\nvar v []int = makeInts()\nvar total = Sum(v...)\ntotal\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}
