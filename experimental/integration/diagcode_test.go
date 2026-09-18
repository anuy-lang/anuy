package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/experimental/semantic"
)

func TestAnalyzeSourceUnknownReadCarriesCodeAndSeverity(t *testing.T) {
	// D-01 reproducer: a read of an unresolved bare identifier.
	result, err := AnalyzeSource("x\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
	if got := result.Diagnostics[0]; got.Code != "ANUY2001" || got.Severity != semantic.SeverityError {
		t.Fatalf("UnknownRead = (%s, %s), want (ANUY2001, Error)", got.Code, got.Severity)
	}
}

func TestAnalyzeSourceReadBeforeInitializationCarriesCode(t *testing.T) {
	result, err := AnalyzeSource("var x int\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
	if got := result.Diagnostics[0]; got.Code != "ANUY3001" || got.Severity != semantic.SeverityError {
		t.Fatalf("ReadBeforeInitialization = (%s, %s), want (ANUY3001, Error)", got.Code, got.Severity)
	}
}

func TestAnalyzeSourceUnsafeMemberAccessCarriesCode(t *testing.T) {
	// R2: an unproven deref of a declared `T?` receiver is an Error.
	result, err := AnalyzeSource("var user User? = findUser()\nuser.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
	if got := result.Diagnostics[0]; got.Code != "ANUY4001" || got.Severity != semantic.SeverityError {
		t.Fatalf("UnsafeMemberAccess = (%s, %s), want (ANUY4001, Error)", got.Code, got.Severity)
	}
}

func TestAnalyzeSourceCascadePairCarriesBothCodes(t *testing.T) {
	// Решение 4 reproducer: one access site hits both dimensions — the
	// uninitialized captured binding is read and derefed inside the closure
	// body. The deref nests into the ReadBeforeInitialization primary
	// (CONTRACTS §2): one published diagnostic, codes on primary+related.
	result, err := AnalyzeSource("var user User?\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v, want one ReadBeforeInitialization primary", result.Diagnostics)
	}
	primary := result.Diagnostics[0]
	if primary.Code != "ANUY3001" || primary.Severity != semantic.SeverityError {
		t.Fatalf("primary = (%s, %s), want (ANUY3001, Error)", primary.Code, primary.Severity)
	}
	if len(primary.Related) != 1 || primary.Related[0].Code != "ANUY4001" || primary.Related[0].Severity != semantic.SeverityError {
		t.Fatalf("related = %#v, want one (ANUY4001, Error)", primary.Related)
	}
}
