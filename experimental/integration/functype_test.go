package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 70 (RFC-019 §6.2/§6.16 v3): kernel checks for func types -
// signature identity ignores parameter names (documentation only), and
// assignability is exact with the RFC-002 outer widening.

func TestFuncTypeSignatureIdentityIgnoresNames(t *testing.T) {
	result, err := AnalyzeSource("var cb func(c int) int = func(x int) int {\nreturn x\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none (names are documentation)", result.Diagnostics)
	}
}

func TestFuncTypeInitializerMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("var cb func(int) int = func(s string) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
	got := result.Diagnostics[0]
	if got.Code != "ANUY7008" || got.Severity != semantic.SeverityError {
		t.Fatalf("FunctionTypeMismatch = (%s, %s), want (ANUY7008, Error)", got.Code, got.Severity)
	}
}

func TestFuncTypeResultShapeMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("var cb func() int = func() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
}

func TestFuncTypeNullableTargetWideningAccepts(t *testing.T) {
	// §6.16 v3: a non-null function value widens to the nullable
	// whole-function type (RFC-002 T -> T?).
	result, err := AnalyzeSource("var cb (func() int)? = func() int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none (widening)", result.Diagnostics)
	}
}

func TestFuncTypeAssignMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("var cb func(int) int = func(v int) int {\nreturn v\n}\ncb = func(s string) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
}

func TestFuncTypeArgumentMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("func Apply(transform func(int) int) int {\nreturn transform(1)\n}\nApply(func(s string) int {\nreturn 1\n})\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
}

func TestFuncTypeArgumentMatchClean(t *testing.T) {
	result, err := AnalyzeSource("func Apply(transform func(int) int) int {\nreturn transform(1)\n}\nApply(func(v int) int {\nreturn v\n})\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

// Story 70 (RFC-019 §6.7, §8.6 semantics): invoking a nullable function
// value requires a live non-nil narrowing fact - the call is the access
// analogue of UnsafeMemberAccess (ANUY4001).
func TestNullableFunctionCallReports(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = findHandler()\ncb()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableFunctionCall")
	got := result.Diagnostics[0]
	if got.Code != "ANUY4011" || got.Severity != semantic.SeverityError {
		t.Fatalf("NullableFunctionCall = (%s, %s), want (ANUY4011, Error)", got.Code, got.Severity)
	}
}

func TestNullableFunctionCallNarrowedClean(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = findHandler()\nif cb != nil {\ncb()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none (narrowed)", result.Diagnostics)
	}
}

// The nullable-call gate stands on the declared-class contract: an
// unproven nullable source does not silently enter a non-null function
// binding - the existing D-1 gate (story 20) fires.
func TestNullableFuncSourceToNonNullReportsD1(t *testing.T) {
	result, err := AnalyzeSource("var cb (func())? = nil\nvar ok func() = cb\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NilToNonNull")
}
