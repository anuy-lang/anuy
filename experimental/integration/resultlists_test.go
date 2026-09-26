package integration

import (
	"testing"
)

// Story 74 (RFC-019 §6.5 v6): plain multi-results are ordinary
// non-fallible functions - the §6.5 Split example works end to end.

func TestPlainMultiResultForwardingClean(t *testing.T) {
	result, err := AnalyzeSource("func Split(v string) (string, string) {\nreturn v, v\n}\nfunc Run() (string, string) {\nreturn Split(\"ab\")\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestNullableFuncResultDeclaresClean(t *testing.T) {
	// The story-73 blocker: `(func() int)?` - the nullable whole-function
	// result with its required parenthesized spelling.
	result, err := AnalyzeSource("func Make() (func() int)? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestFallibleMultiResultStrictContractIntact(t *testing.T) {
	// The story 35/36 strict contract is untouched: the fallible shape
	// still requires the error to be handled (ANUY6006 for a discarded
	// fallible result in a destructuring is story 36 machinery; here the
	// unchecked error lint may fire - only validity codes are asserted).
	result, err := AnalyzeSource("func Load() (int, error?) {\nreturn 0, nil\n}\nvar a, e = Load()\nif e != nil {\n}\na\ne\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range result.Diagnostics {
		if d.Severity == "Error" {
			t.Fatalf("unexpected Error diagnostic in the guarded fallible shape: %v", d)
		}
	}
}
