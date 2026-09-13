package parser

import (
	"errors"
	"testing"
)

func requireCategory(t *testing.T, err error, want ErrorCategory) {
	t.Helper()
	var perr *Error
	if !errors.As(err, &perr) {
		t.Fatalf("err = %v, want parser error", err)
	}
	if perr.Category != want {
		t.Fatalf("category = %q, want %q", perr.Category, want)
	}
}

func TestParseVarAndAssignmentWithSpans(t *testing.T) {
	program, err := Parse("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 2 || program.Statements[0].Kind != Var || program.Statements[1].Kind != Assign {
		t.Fatalf("program = %#v", program)
	}
	if program.Statements[1].Span.Start != 10 {
		t.Fatalf("assignment span = %#v", program.Statements[1].Span)
	}
}

func TestParseAcceptsDeclarationForms(t *testing.T) {
	sources := []string{
		"var x int",
		"var x int = 5",
		"var x = 5",
		"var u User?",
		"var x, y = f(), g()",
	}
	for _, source := range sources {
		program, err := Parse(source + "\n")
		if err != nil {
			t.Fatalf("Parse(%q): %v", source, err)
		}
		if len(program.Statements) != 1 || program.Statements[0].Kind != Var {
			t.Fatalf("Parse(%q): program = %#v", source, program)
		}
	}
}

func TestParseAcceptsMultipleAssignment(t *testing.T) {
	program, err := Parse("x, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Kind != Assign || len(statement.Names) != 2 || len(statement.Values) != 2 {
		t.Fatalf("statement = %#v", statement)
	}
	if statement.Values[0] != "y" || statement.Values[1] != "x" {
		t.Fatalf("values = %#v", statement.Values)
	}
}

func TestParseCapturesValueSourceText(t *testing.T) {
	program, err := Parse("x = count + 1\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Values[0] != "count + 1" {
		t.Fatalf("values = %#v", statement.Values)
	}
	if len(statement.ValueIdents[0]) != 1 || statement.ValueIdents[0][0] != "count" {
		t.Fatalf("value idents = %#v", statement.ValueIdents)
	}
}

func TestParseRejectsBareDeclaration(t *testing.T) {
	for _, source := range []string{"var x", "var x, y"} {
		_, err := Parse(source + "\n")
		if err == nil {
			t.Fatalf("Parse(%q) accepted bare declaration", source)
		}
		requireCategory(t, err, BareDeclaration)
	}
}

func TestParseRejectsTypedMultipleDeclaration(t *testing.T) {
	_, err := Parse("var x, y int = 1, 2\n")
	if err == nil {
		t.Fatal("typed multiple declaration accepted")
	}
	requireCategory(t, err, TypedMultipleDeclaration)
}

func TestParseRejectsShortDeclaration(t *testing.T) {
	_, err := Parse("x := 1\n")
	if err == nil {
		t.Fatal("short declaration accepted")
	}
	requireCategory(t, err, ShortDeclaration)
}

func TestParseRejectsArityMismatch(t *testing.T) {
	_, err := Parse("x, y = 1, 2, 3\n")
	if err == nil {
		t.Fatal("arity mismatch accepted")
	}
	requireCategory(t, err, ArityMismatch)
}

func TestParseAcceptsSingleValueForMultipleTargets(t *testing.T) {
	if _, err := Parse("x, y = f()\n"); err != nil {
		t.Fatalf("multi-value call form rejected: %v", err)
	}
}

func TestParseRejectsDuplicateAssignmentTargets(t *testing.T) {
	_, err := Parse("x, x = 1, 2\n")
	if err == nil {
		t.Fatal("duplicate assignment targets accepted")
	}
	requireCategory(t, err, DuplicateAssignmentTarget)
}

func TestParseRejectsBlankIdentifier(t *testing.T) {
	for _, source := range []string{"var v, _ = f()", "_ = 1"} {
		_, err := Parse(source + "\n")
		if err == nil {
			t.Fatalf("Parse(%q) accepted blank identifier", source)
		}
		requireCategory(t, err, UnsupportedSyntax)
	}
}

func TestParseRejectsUnsupportedSyntax(t *testing.T) {
	if _, err := Parse("func main() {}\n"); err == nil {
		t.Fatal("unsupported syntax accepted")
	}
}

func TestParseAcceptsIfWithBlock(t *testing.T) {
	program, err := Parse("if ready {\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Kind != If || statement.Cond != "ready" || len(statement.CondIdents) != 1 {
		t.Fatalf("statement = %#v", statement)
	}
	if len(statement.Body) != 1 || statement.Body[0].Kind != Assign || statement.Else != nil {
		t.Fatalf("branches = %#v", statement)
	}
}

func TestParseAcceptsIfElseWithConditions(t *testing.T) {
	program, err := Parse("if x != nil {\nx = 1\n} else {\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Cond != "x != nil" || len(statement.Body) != 1 || len(statement.Else) != 1 {
		t.Fatalf("statement = %#v", statement)
	}
}

func TestParseAcceptsNestedIf(t *testing.T) {
	program, err := Parse("if a {\nif b {\nx = 1\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	outer := program.Statements[0]
	if outer.Kind != If || len(outer.Body) != 1 || outer.Body[0].Kind != If || len(outer.Body[0].Body) != 1 {
		t.Fatalf("statement = %#v", outer)
	}
}

func TestParseRejectsIfWithoutBlock(t *testing.T) {
	_, err := Parse("if ready\n")
	if err == nil {
		t.Fatal("if without block accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsElseIf(t *testing.T) {
	_, err := Parse("if ready {\nx = 1\n} else if other {\nx = 2\n}\n")
	if err == nil {
		t.Fatal("else if accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsUnbalancedBlock(t *testing.T) {
	if _, err := Parse("if ready {\nx = 1\n"); err == nil {
		t.Fatal("unbalanced block accepted")
	}
}

func TestParseRejectsUnexpectedClosingBrace(t *testing.T) {
	if _, err := Parse("}\n"); err == nil {
		t.Fatal("unexpected closing brace accepted")
	}
}
