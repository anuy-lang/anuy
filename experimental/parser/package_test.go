package parser

import (
	"errors"
	"testing"
)

// rejectPackageAssert asserts the parse rejects with the ANUY1001 code.
func rejectPackageAssert(t *testing.T, source string) {
	t.Helper()
	_, err := Parse(source)
	if err == nil {
		t.Fatalf("Parse(%q) accepted an invalid package clause", source)
	}
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("Parse(%q) = %v, want ANUY1001", source, err)
	}
}

// Story 61 (RFC-010 §6.1.7, RFC-015 §6.1 v3): a .anuy file MAY open with
// a Go-compatible package clause; the declared name becomes the generated
// package name (RFC-009 §6.10 contract surface).

func TestPackageClauseParses(t *testing.T) {
	program, err := Parse("package server\n\nvar x int = 1\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if program.Package != "server" {
		t.Fatalf("Package = %q, want %q", program.Package, "server")
	}
	if program.PackageSpan.Start != 0 {
		t.Fatalf("PackageSpan = %+v, want the clause at 1:1", program.PackageSpan)
	}
	if len(program.Statements) != 2 {
		t.Fatalf("statements = %d, want the declarations after the clause", len(program.Statements))
	}
}

// Leading comments are part of the file header in Go - the clause is the
// first non-comment line.
func TestPackageClauseAfterComment(t *testing.T) {
	program, err := Parse("// service layer\npackage server\nvar x int = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if program.Package != "server" {
		t.Fatalf("Package = %q, want %q", program.Package, "server")
	}
}

// The clause outside the first statement position is a syntax reject -
// Go package clauses are file-opening, and a second clause is a
// duplicate (RFC-015 §6.1 v3).
func TestPackageClausePositionRejects(t *testing.T) {
	for _, source := range []string{
		"var x int = 1\npackage server\n", // not the first line
		"package a\npackage b\n",          // duplicate clause
	} {
		rejectPackageAssert(t, source)
	}
}

// The name must be a Go identifier outside the Anuy reserved words
// (RFC-015 §6.1 v3). A Go keyword outside the Anuy set (`range`) parses
// here and is caught at check by §6.4.13 (ANUY9001) - same split as
// binding identifiers.
func TestPackageClauseNameRejects(t *testing.T) {
	for _, source := range []string{
		"package\n",     // no name
		"package 9x\n",  // not an identifier
		"package a.b\n", // not an identifier
		"package if\n",  // Anuy reserved
	} {
		rejectPackageAssert(t, source)
	}
}

// A file without the clause stays in the transitional single-file mode:
// the default name keeps every existing golden pin stable (RFC-015 §6.1
// v3, default `fixture`).
func TestPackageClauseOptional(t *testing.T) {
	program, err := Parse("var x int = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if program.Package != "fixture" {
		t.Fatalf("Package = %q, want the transitional default %q", program.Package, "fixture")
	}
}
