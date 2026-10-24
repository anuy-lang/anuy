package integration

import (
	"testing"
)

// Story 80 (RFC-024 §6.23 v4, RFC-018 §6.29 v2): package constants
// pre-register into the package namespace but are not package variables -
// no initialization steps, no dependency edges (§6.2 RFC-024), reads are
// modeled by the entry seeding.

func TestConstantReadSeesEntryState(t *testing.T) {
	result, err := AnalyzeSource("const MaxRetries = 5\nMaxRetries")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// Declaration order is not observable: a body declared before the
// constant still reads it.
func TestConstantReadBeforeDeclaration(t *testing.T) {
	result, err := AnalyzeSource("func shout() {\nMaxRetries\n}\nconst MaxRetries = 5")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.2: a reference to a compile-time constant creates no
// package-variable initialization dependency - the forward reference to
// the constant stays cycle-free (the story 78 graph ignores it).
func TestConstantReferenceCreatesNoDependency(t *testing.T) {
	result, err := AnalyzeSource("var x = MaxRetries\nfunc shout() {\nMaxRetries\n}\nconst MaxRetries = 5\nx\nshout()")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none (dep vacuity)", joinedCodes(result))
	}
}

// §6.5/§8.17 v2 RFC-018: an assignment to a constant rejects.
func TestAssignmentToConstantRejects(t *testing.T) {
	result, err := AnalyzeSource("const MaxRetries = 5\nMaxRetries = 10")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2004") {
		t.Fatalf("diagnostics = %s, want one ANUY2004", joinedCodes(result))
	}
}

// The immutability check runs from function bodies too.
func TestAssignmentToConstantFromBodyRejects(t *testing.T) {
	result, err := AnalyzeSource("const MaxRetries = 5\nfunc f() {\nMaxRetries = 1\n}\nf()")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2004") {
		t.Fatalf("diagnostics = %s, want one ANUY2004", joinedCodes(result))
	}
}

// §8.17 v2: the immutability verdict precedes the conditional-shape
// verdict - the constant cannot be assigned at all.
func TestConstantTargetCommaOkRejectsWithImmutability(t *testing.T) {
	result, err := AnalyzeSource("const MaxRetries = 5\nvar found = false\nMaxRetries, found = users[id]")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2004") {
		t.Fatalf("diagnostics = %s, want one ANUY2004 (before ANUY3006)", joinedCodes(result))
	}
}

// Multi-target: one constant target is enough to reject the assignment.
func TestMultiTargetWithConstantRejects(t *testing.T) {
	result, err := AnalyzeSource("const MaxRetries = 5\nvar d = 0\nMaxRetries, d = 1, 2")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2004") {
		t.Fatalf("diagnostics = %s, want one ANUY2004", joinedCodes(result))
	}
}

// Duplicate constant names reject with the ordinary redeclaration
// verdict, in source order either way around.
func TestConstantDuplicatesReject(t *testing.T) {
	result, err := AnalyzeSource("const C = 5\nconst C = 6")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY2003") {
		t.Fatalf("diagnostics = %s, want ANUY2003", joinedCodes(result))
	}
	result, err = AnalyzeSource("const C = 5\nvar C = 6")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY2003") {
		t.Fatalf("diagnostics = %s, want ANUY2003", joinedCodes(result))
	}
}

// A local binding named like a package constant shadows it - ordinary
// RFC-003 dataflow.
func TestLocalShadowsConstant(t *testing.T) {
	result, err := AnalyzeSource("const C = 5\nfunc f() {\nvar C = 1\nC = 2\n}\nf()")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none (local shadowing)", joinedCodes(result))
	}
}

// The typed form registers the same way.
func TestTypedConstantRead(t *testing.T) {
	result, err := AnalyzeSource("const Max int = 5\nMax")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}
