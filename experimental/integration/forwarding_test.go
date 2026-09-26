package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 76 (RFC-005 §6.7.1, RFC-019 §6.6): direct tail forwarding of the
// entire fallible result - the full result protocol (count + positions)
// verifies against the enclosing declaration; the D-7 literal-nil
// contract does not apply to a forwarded call.

func TestFallibleForwardingClean(t *testing.T) {
	result, err := AnalyzeSource("func ReadData() (int, error?) {\nreturn 1, nil\n}\nfunc Load() (int, error?) {\nreturn ReadData()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestFallibleForwardingProtocolMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("func ReadText() (string, error?) {\nreturn \"\", nil\n}\nfunc Load() (int, error?) {\nreturn ReadText()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ArityMismatch")
	got := result.Diagnostics[0]
	if got.Code != "ANUY1006" || got.Severity != semantic.SeverityError {
		t.Fatalf("protocol mismatch = (%s, %s), want (ANUY1006, Error)", got.Code, got.Severity)
	}
}

func TestFallibleForwardingArityMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("func ReadBoth() (int, int, error?) {\nreturn 1, 2, nil\n}\nfunc Load() (int, error?) {\nreturn ReadBoth()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ArityMismatch")
}

func TestNonFallibleForwardingProtocolMismatchReports(t *testing.T) {
	// The story 74 non-fallible forwarding joins the same protocol check.
	result, err := AnalyzeSource("func Split(v string) (string, string) {\nreturn v, v\n}\nfunc Run() (int, int) {\nreturn Split(\"ab\")\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ArityMismatch")
}

func TestForwardingUnknownCalleeTolerant(t *testing.T) {
	// An unknown callee's protocol is unknown - tolerance; the go
	// type-check stage verifies (story 64).
	result, err := AnalyzeSource("func Load() (int, error?) {\nreturn Mystery()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none (tolerance)", result.Diagnostics)
	}
}

func TestStrictLiteralReturnContractIntact(t *testing.T) {
	// The D-7 contract keeps its literal-nil form for ordinary strict
	// returns (story 35) - a forwarded call is the exempt shape.
	result, err := AnalyzeSource("func ReadData() (int, error?) {\nreturn 1, nil\n}\nfunc Load() (int, error?) {\nreturn 1, err\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range result.Diagnostics {
		if d.Category == "MixedReturn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("diagnostics = %v, want MixedReturn for the non-forwarding strict return", result.Diagnostics)
	}
}
