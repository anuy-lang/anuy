package lowering

import (
	"strings"
	"testing"
)

// Story 82 (RFC-018 §6.29 v3): the lowering is unchanged - constant
// expressions lower verbatim and the Go toolchain folds them; the
// checker's evaluation is a source-level verdict, not a rewrite.
func TestLowerConstantReferenceVerbatim(t *testing.T) {
	got, err := Lower("const A = 5\nconst B = A + 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n//line source.anuy:1\nconst A = 5\n//line source.anuy:2\nconst B = A + 1\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "= 6") {
		t.Fatalf("the checker's evaluation must not rewrite the declaration:\n%s", got)
	}
}
