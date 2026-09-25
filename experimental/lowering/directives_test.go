package lowering

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Story 59 (RFC-010 §6.9.8): generated Go carries //line directives, so
// the Go toolchain itself reports positions against the Anuy source. The
// directive is honored only at the start of a line — the emission is
// always at column zero.

func TestLowerFileAnchorsStatementsToSourceLines(t *testing.T) {
	got, err := LowerFile("prog.anuy", "var x int = 1\nx = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n//line prog.anuy:1\n\tvar x int = 1\n//line prog.anuy:2\n\tx = 2\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("LowerFile() = %q, want %q", got, want)
	}
}

func TestLowerShimAnchorsToCanonicalName(t *testing.T) {
	// The pathless entry keeps a canonical spelling so the existing
	// one-text pins stay deterministic (§6.9.8: one lowering, one text).
	got, err := Lower("var x int\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "//line source.anuy:1\n") {
		t.Fatalf("Lower() misses the canonical-path directive:\n%s", got)
	}
}

func TestLowerFileAnchorsDeclarationsAndWrappers(t *testing.T) {
	// The foreign-entry wrapper (story 42) and its `__anuy_` native body
	// are synthetic declarations — both anchor to the owning source
	// declaration (§6.9.6 blame, §6.9.8); body statements carry their own
	// source lines.
	src := "type User struct {\nid int\n}\nfunc UseUser(u *User) {\nu.id = 2\n}\n"
	got, err := LowerFile("prog.anuy", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"//line prog.anuy:1\ntype User struct {",
		"//line prog.anuy:4\nfunc UseUser(u *User) {",
		"//line prog.anuy:5\n\tu.id = 2",
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("LowerFile() misses %q:\n%s", marker, got)
		}
	}
}

func TestLowerFileAnchorsValidatorsToTheirType(t *testing.T) {
	// Aggregate validators are synthetic (story 43) — they anchor to the
	// type declaration that owns them (§6.9.8), not to the physical line
	// they happen to occupy in the generated file.
	data, err := os.ReadFile(filepath.Join("..", "fixtures", "lowering-core", "accept", "foreign-entry-aggregate.anuy"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := LowerFile("ag.anuy", string(data))
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		// Badge is declared on line 10, User on line 5 of the fixture.
		"//line ag.anuy:10\nfunc __anuy_validateBadge",
		"//line ag.anuy:5\nfunc __anuy_validateUser",
	} {
		if !strings.Contains(got, marker) {
			t.Fatalf("LowerFile() misses %q:\n%s", marker, got)
		}
	}
}
