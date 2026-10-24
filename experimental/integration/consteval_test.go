package integration

import (
	"testing"
)

// Story 82 (RFC-018 §6.29 v3): the constant environment - evaluation of
// constant expressions at the checker, forward references included.

func TestConstantToConstantEvaluates(t *testing.T) {
	result, err := AnalyzeSource("const A = 5\nconst B = A + 1\nB")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// Lazy resolution: a forward reference evaluates.
func TestConstantForwardReferenceEvaluates(t *testing.T) {
	result, err := AnalyzeSource("const B = A + 1\nconst A = 5\nB")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.5: an unknown name in a constant initializer is the undefined-name
// read (ANUY2001).
func TestConstantUnknownReferenceRejects(t *testing.T) {
	result, err := AnalyzeSource("const B = Ghost + 1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2001") {
		t.Fatalf("diagnostics = %s, want one ANUY2001", joinedCodes(result))
	}
}

// §6.5: a reference to a non-constant binding cannot initialize a
// constant (ANUY2007).
func TestConstantVarReferenceRejects(t *testing.T) {
	result, err := AnalyzeSource("var v = 1\nconst C = v")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || string(result.Diagnostics[0].Code) != "ANUY2007" {
		t.Fatalf("diagnostics = %s, want one ANUY2007", joinedCodes(result))
	}
}

// §8.4/§6.9: a typed constant operand keeps the result typed - the
// overflow is a compile-time error, never a wrap.
func TestConstantTypedOverflowRejects(t *testing.T) {
	result, err := AnalyzeSource("const X uint8 = 255\nconst Y = X + 1")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2005") {
		t.Fatalf("diagnostics = %s, want one ANUY2005", joinedCodes(result))
	}
}

// §8.1: a typed constant declaration checks representability.
func TestTypedConstantNotRepresentableRejects(t *testing.T) {
	result, err := AnalyzeSource("const X uint8 = 300")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2005") {
		t.Fatalf("diagnostics = %s, want one ANUY2005", joinedCodes(result))
	}
}

// §8.5: constant division by zero is a compile-time error.
func TestConstantDivByZeroRejects(t *testing.T) {
	result, err := AnalyzeSource("const X = 1 / 0")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2006") {
		t.Fatalf("diagnostics = %s, want one ANUY2006", joinedCodes(result))
	}
}

// §6.4/§8.1: a typed declaration contextually converts the constant -
// representability is compile-time checked.
func TestTypedVarLiteralRepresentability(t *testing.T) {
	result, err := AnalyzeSource("var s int8 = 1000")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY2005") {
		t.Fatalf("diagnostics = %s, want one ANUY2005", joinedCodes(result))
	}
	result, err = AnalyzeSource("var s int8 = 100\ns")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", joinedCodes(result))
	}
}

// §6.4: a constant never arrives nil - the inferred binding from a
// constant is non-null, so a nil assignment is the D-1 violation.
func TestInferredVarFromConstantNonNull(t *testing.T) {
	result, err := AnalyzeSource("const C = 5\nvar x = C\nx = nil")
	if err != nil {
		t.Fatal(err)
	}
	if !hasOnlyCode(result, "ANUY4005") {
		t.Fatalf("diagnostics = %s, want one ANUY4005 (D-1 via the NonNull class)", joinedCodes(result))
	}
}
