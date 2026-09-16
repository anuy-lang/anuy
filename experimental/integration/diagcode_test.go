package integration

import (
	"testing"

	"github.com/san-smith/anuy/experimental/semantic"
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
	// body. Until the cascade suppression (1-6-2-1) both report
	// independently; this pins their codes on the pair.
	result, err := AnalyzeSource("var user User?\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("result = %#v, want the ReadBeforeInitialization + UnsafeMemberAccess pair", result.Diagnostics)
	}
	codes := map[semantic.Code]bool{}
	for _, d := range result.Diagnostics {
		codes[d.Code] = true
	}
	if !codes["ANUY3001"] || !codes["ANUY4001"] {
		t.Fatalf("codes = %v, want ANUY3001 + ANUY4001", codes)
	}
}
