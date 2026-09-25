package lowering

import (
	"strings"
	"testing"
)

// Story 63 (RFC-015 §6.15 v5): the declared imports become the generated
// import block - sorted, deduplicated, anchored to their source lines.

func TestLowerFileImportsEmitSortedAnchored(t *testing.T) {
	got, err := LowerFile("prog.anuy", "package main\nimport \"strings\"\nimport \"fmt\"\nvar up = strings.ToUpper(\"x\")\nfmt.Println(up)\n")
	if err != nil {
		t.Fatal(err)
	}
	// Source order is strings(2), fmt(3) - the emission is sorted.
	for _, want := range []string{
		"//line prog.anuy:3\nimport \"fmt\"\n",
		"//line prog.anuy:2\nimport \"strings\"\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("LowerFile() misses %q:\n%s", want, got)
		}
	}
	if strings.Index(got, "import \"fmt\"") > strings.Index(got, "import \"strings\"") {
		t.Fatalf("imports are not sorted:\n%s", got)
	}
}

// The package union deduplicates shared imports; the first occurrence in
// file order carries the anchor.
func TestLowerPackageImportsUnion(t *testing.T) {
	got, err := LowerPackage([]File{
		{Path: "a.anuy", Source: "package p\nimport \"fmt\"\nfunc A() int {\nreturn 1\n}\n"},
		{Path: "b.anuy", Source: "package p\nimport \"fmt\"\nimport \"strings\"\nfunc B() int {\nreturn strings.Count(\"x\", \"x\")\n}\n"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(got, "import \"fmt\"") != 1 {
		t.Fatalf("shared import duplicated:\n%s", got)
	}
	if !strings.Contains(got, "//line a.anuy:2\nimport \"fmt\"\n") {
		t.Fatalf("fmt misses the first-occurrence anchor:\n%s", got)
	}
	if !strings.Contains(got, "//line b.anuy:3\nimport \"strings\"\n") {
		t.Fatalf("strings misses its anchor:\n%s", got)
	}
}
