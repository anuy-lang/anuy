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
	if statement.Values[0].Text != "y" || statement.Values[1].Text != "x" {
		t.Fatalf("values = %#v", statement.Values)
	}
}

func TestParseCapturesValueSourceText(t *testing.T) {
	program, err := Parse("x = count + 1\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if value.Text != "count + 1" {
		t.Fatalf("value = %#v", value)
	}
	if len(value.Idents) != 1 || value.Idents[0] != "count" {
		t.Fatalf("idents = %#v", value.Idents)
	}
}

func TestParseAcceptsClosureLiteral(t *testing.T) {
	program, err := Parse("var increment = func() {\ncount = count + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if value.Closure == nil {
		t.Fatalf("value = %#v, want closure", value)
	}
	if len(value.Closure.Params) != 0 || len(value.Closure.Body) != 1 || value.Closure.Body[0].Kind != Assign {
		t.Fatalf("closure = %#v", value.Closure)
	}
}

func TestParseAcceptsClosureWithTypedParams(t *testing.T) {
	program, err := Parse("var f = func(user User, ids []int) {\nuser\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	cl := program.Statements[0].Values[0].Closure
	if cl == nil || len(cl.Params) != 2 {
		t.Fatalf("closure = %#v", cl)
	}
	if cl.Params[0].Name != "user" || cl.Params[0].Type != "User" || cl.Params[1].Type != "[]int" {
		t.Fatalf("params = %#v", cl.Params)
	}
}

func TestParseRejectsClosureWithMultipleBindings(t *testing.T) {
	_, err := Parse("var f, g = func() {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("closure with multiple bindings accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsClosureWithUntypedParam(t *testing.T) {
	_, err := Parse("var f = func(x) {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("untyped closure parameter accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsClosureInCondition(t *testing.T) {
	_, err := Parse("if func() {\nreturn\n} {\nx = 1\n}\n")
	if err == nil {
		t.Fatal("closure in condition accepted")
	}
	requireCategory(t, err, UnsupportedSyntax)
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
