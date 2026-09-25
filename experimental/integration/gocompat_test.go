package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 60 (RFC-010 §6.4.13): identifiers emitted verbatim into generated
// Go must not collide with Go reserved words - check reports them before
// any Go-toolchain invocation (§6.4.9).

func findGoReserved(t *testing.T, diags []semantic.Diagnostic) semantic.Diagnostic {
	t.Helper()
	for _, d := range diags {
		if d.Code == "ANUY9001" {
			return d
		}
	}
	t.Fatalf("no ANUY9001 among %d diagnostics", len(diags))
	return semantic.Diagnostic{}
}

// F-59-5: `range` is not reserved in Anuy but is a Go keyword - the
// collision becomes a check-stage Error at the declaration.
func TestGoReservedIdentifierBinding(t *testing.T) {
	result, err := AnalyzeSource("var range = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	diag := findGoReserved(t, result.Diagnostics)
	if diag.Severity != semantic.SeverityError || diag.Span.Start != 0 {
		t.Fatalf("diag = %+v, want ANUY9001 Error at 1:1", diag)
	}
}

// The field reaches generated Go verbatim (`v.range`); the AST carries the
// field span - the exact construct is blamed (§6.2.18). `type User struct
// {\n` is 19 bytes, so the second-line field spans offsets 19..28.
func TestGoReservedIdentifierStructField(t *testing.T) {
	result, err := AnalyzeSource("type User struct {\nrange int\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	diag := findGoReserved(t, result.Diagnostics)
	if diag.Span.Start != 19 || diag.Span.End != 28 {
		t.Fatalf("span = %+v, want the exact field span at 2:1", diag.Span)
	}
}

// Params carry no span in the AST - the statement fallback applies
// (§6.2.18): a coarse span beats a zero one.
func TestGoReservedIdentifierClosureParam(t *testing.T) {
	result, err := AnalyzeSource("var f = func(range int) {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	diag := findGoReserved(t, result.Diagnostics)
	if diag.Span.Start != 0 {
		t.Fatalf("span = %+v, want the statement fallback at 1:1", diag.Span)
	}
}

// Names that never reach Go verbatim stay clean: `rangeX` is not a
// keyword, and enum variants are prefixed by the generated-name contract
// (`Trange`, RFC-009 §6.10).
func TestGoReservedIdentifierExemptions(t *testing.T) {
	result, err := AnalyzeSource("var rangeX = 1\ntype T enum {\nrange\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range result.Diagnostics {
		if d.Code == "ANUY9001" {
			t.Fatalf("unexpected ANUY9001: %+v", d)
		}
	}
}
