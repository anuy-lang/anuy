package parser

import (
	"errors"
	"testing"
)

// Story 63 (RFC-015 §6.15 v5): the header import clause - single-line
// `import "path"` after the package clause; the declared paths become
// the generated import block.

func TestImportClauseParses(t *testing.T) {
	program, err := Parse("package main\nimport \"fmt\"\nimport \"strings\"\nvar x = 1\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Imports) != 2 || program.Imports[0].Path != "fmt" || program.Imports[1].Path != "strings" {
		t.Fatalf("Imports = %+v, want [fmt strings]", program.Imports)
	}
	if program.Imports[0].Span.Start != len("package main\n") {
		t.Fatalf("span = %+v, want the clause on line 2", program.Imports[0].Span)
	}
	if len(program.Statements) != 2 {
		t.Fatalf("statements = %d, want the declarations after the clause", len(program.Statements))
	}
}

// Comments and blank lines inside the header do not end the clause run.
func TestImportClauseAfterComments(t *testing.T) {
	program, err := Parse("package main\n// stdlib\nimport \"fmt\"\n\nimport \"strings\"\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Imports) != 2 {
		t.Fatalf("Imports = %+v, want two entries", program.Imports)
	}
}

func rejectImportAssert(t *testing.T, source string) {
	t.Helper()
	_, err := Parse(source)
	if err == nil {
		t.Fatalf("Parse(%q) accepted an invalid import clause", source)
	}
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("Parse(%q) = %v, want ANUY1001", source, err)
	}
}

// The clause is file-header only: before the package clause or after
// statements rejects.
func TestImportClausePlacementRejects(t *testing.T) {
	for _, source := range []string{
		"import \"fmt\"\npackage main\n",            // before the package clause
		"package main\nvar x = 1\nimport \"fmt\"\n", // after statements
	} {
		rejectImportAssert(t, source)
	}
}

// Only the plain `import "path"` form is in the slice: named, dot and
// blank forms reject, as does a bare word instead of a path.
func TestImportClauseFormRejects(t *testing.T) {
	for _, source := range []string{
		"package main\nimport\n",            // no path
		"package main\nimport fmt \"io\"\n", // named form
		"package main\nimport . \"io\"\n",   // dot form
		"package main\nimport _ \"io\"\n",   // blank form
		"package main\nimport io\n",         // bare word, not a path
	} {
		rejectImportAssert(t, source)
	}
}

// With imports in the language, `import` is a keyword and stops being a
// legal binding name.
func TestImportBecomesReserved(t *testing.T) {
	if _, err := Parse("package main\nvar import = 1\n"); err == nil {
		t.Fatal("Parse accepted `import` as a binding name")
	}
}
