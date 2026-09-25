package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 64 (RFC-003 resolution, §6.4.9 RFC-010): value reads resolve -
// an unknown identifier in an initializer (or any value position) is an
// ANUY2001 Error, not a silent skip that only go build catches
// (F-62-1/F-62-5).

func hasUnknownRead(t *testing.T, result Result) semantic.Diagnostic {
	t.Helper()
	for _, d := range result.Diagnostics {
		if d.Code == "ANUY2001" {
			return d
		}
	}
	t.Fatalf("no ANUY2001 among %d diagnostics", len(result.Diagnostics))
	return semantic.Diagnostic{}
}

func TestInitializerUnknownIdent(t *testing.T) {
	result, err := AnalyzeSource("package p\nvar x = ghost + 1\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	diag := hasUnknownRead(t, result)
	// The value span covers the initializer expression on line 2.
	if diag.Span.Start == 0 {
		t.Fatalf("diag span = %+v, want the initializer value span", diag.Span)
	}
}

func TestInitializerUnknownCallee(t *testing.T) {
	result, err := AnalyzeSource("package p\nfunc Real() int {\nreturn 1\n}\nvar x = Ghost() + Real()\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	hasUnknownRead(t, result)
}

func TestInitializerUnknownLocal(t *testing.T) {
	result, err := AnalyzeSource("package p\nfunc f() int {\nvar z = ghost\nreturn z\n}\nf()\n")
	if err != nil {
		t.Fatal(err)
	}
	hasUnknownRead(t, result)
}

// The exemption namespaces stay clean: an import qualifier is not a
// binding read (story 63 pass-through), and an enum variant constant is
// a generated constant, not a binding (RFC-006 §6.1).
func TestValueReadExemptions(t *testing.T) {
	sources := []string{
		"package main\nimport \"strings\"\nvar up = strings.ToUpper(\"x\")\nup\n",
		"package p\ntype Role enum {\nmember\nadmin\n}\nvar r = Rolemember\nr\n",
	}
	for _, source := range sources {
		result, err := AnalyzeSource(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range result.Diagnostics {
			if d.Code == "ANUY2001" {
				t.Fatalf("unexpected ANUY2001 for %q: %+v", source, d)
			}
		}
	}
}
