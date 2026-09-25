package lowering

import (
	"strings"
	"testing"
)

// Story 61 (RFC-010 §6.1.7): the declared package clause becomes the
// generated package name; the absence stays in the transitional
// single-file default (RFC-015 §6.1 v3).

func TestLowerFileDeclaredPackage(t *testing.T) {
	got, err := LowerFile("prog.anuy", "package server\nvar x int = 1\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "package server\n") {
		t.Fatalf("LowerFile() misses the declared package name:\n%s", got)
	}
}
