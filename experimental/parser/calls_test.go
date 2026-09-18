package parser

import (
	"testing"

	"github.com/anuy-lang/anuy/experimental/semantic"
)

func parseSource(t *testing.T, source string) Program {
	t.Helper()
	program, err := Parse(source)
	if err != nil {
		t.Fatalf("parse %q: %v", source, err)
	}
	return program
}

func parseReject(t *testing.T, source string) *Error {
	t.Helper()
	_, err := Parse(source)
	perr, ok := err.(*Error)
	if !ok {
		t.Fatalf("parse %q: err = %v, want *Error", source, err)
	}
	return perr
}

func TestParseFunctionDeclaration(t *testing.T) {
	program := parseSource(t, "func greet() {\nprint(\"hi\")\n}\n")
	stmt := program.Statements[0]
	if stmt.Kind != Function || len(stmt.Names) != 1 || stmt.Names[0] != "greet" {
		t.Fatalf("statement = %#v, want Function greet", stmt)
	}
	if stmt.Closure == nil || len(stmt.Closure.Params) != 0 || len(stmt.Closure.Body) != 1 {
		t.Fatalf("closure = %#v, want empty params and one body statement", stmt.Closure)
	}
	body := stmt.Closure.Body[0]
	if body.Kind != Call || len(body.Values) != 1 || body.Values[0].Text != "\"hi\"" {
		t.Fatalf("body = %#v, want print with a literal argument", stmt.Closure.Body)
	}
}

func TestParseFunctionDeclarationWithParams(t *testing.T) {
	program := parseSource(t, "func process(user User, n int) {\n}\n")
	params := program.Statements[0].Closure.Params
	if len(params) != 2 || params[0].Name != "user" || params[0].Type != "User" || params[1].Name != "n" || params[1].Type != "int" {
		t.Fatalf("params = %#v, want (user User, n int)", params)
	}
}

func TestParseSelfRecursiveDeclaration(t *testing.T) {
	program := parseSource(t, "func f() {\nf()\n}\n")
	body := program.Statements[0].Closure.Body[0]
	if body.Kind != Call || body.Call == nil || body.Call.Receiver != "f" {
		t.Fatalf("body = %#v, want a recursive call of f", program.Statements[0].Closure.Body)
	}
}

func TestParseCallWithArguments(t *testing.T) {
	program := parseSource(t, "print(x)\n")
	stmt := program.Statements[0]
	if stmt.Kind != Call || stmt.Call == nil || stmt.Call.Receiver != "print" {
		t.Fatalf("statement = %#v, want a call of print", stmt)
	}
	if len(stmt.Values) != 1 || len(stmt.Values[0].Idents) != 1 || stmt.Values[0].Idents[0] != "x" {
		t.Fatalf("arguments = %#v, want one argument referencing x", stmt.Values)
	}
}

func TestParseCallWithMultipleArguments(t *testing.T) {
	program := parseSource(t, "log(a, \"done\")\n")
	values := program.Statements[0].Values
	if len(values) != 2 || len(values[0].Idents) != 1 || values[0].Idents[0] != "a" || values[1].Text != "\"done\"" {
		t.Fatalf("arguments = %#v, want (a, \"done\")", values)
	}
}

func TestParseNavigationCallWithClosureArgument(t *testing.T) {
	// RFC-003 §41: the closure argument spans lines; the call closes on the
	// block closer line (`})`).
	program := parseSource(t, "users.forEach(func(user User) {\nprint(user)\n})\n")
	stmt := program.Statements[0]
	if stmt.Kind != Call || stmt.Call == nil || stmt.Call.Receiver != "users" {
		t.Fatalf("statement = %#v, want a call on users", stmt)
	}
	if len(stmt.Call.Segments) != 1 || stmt.Call.Segments[0].Name != "forEach" || !stmt.Call.Segments[0].Call {
		t.Fatalf("segments = %#v, want one forEach call segment", stmt.Call.Segments)
	}
	if len(stmt.Values) != 1 || stmt.Values[0].Closure == nil {
		t.Fatalf("arguments = %#v, want one closure argument", stmt.Values)
	}
	closure := stmt.Values[0].Closure
	if len(closure.Params) != 1 || closure.Params[0].Name != "user" || len(closure.Body) != 1 {
		t.Fatalf("closure = %#v, want one param and one body statement", closure)
	}
}

func TestParseNavigationCallWithArgumentsAfterClosure(t *testing.T) {
	program := parseSource(t, "users.forEach(func(user User) {\n}, other)\n")
	values := program.Statements[0].Values
	if len(values) != 2 || values[0].Closure == nil || len(values[1].Idents) != 1 || values[1].Idents[0] != "other" {
		t.Fatalf("arguments = %#v, want (closure, other)", values)
	}
}

func TestParseReturnStatement(t *testing.T) {
	program := parseSource(t, "return\n")
	if program.Statements[0].Kind != Return {
		t.Fatalf("statement = %#v, want Return", program.Statements[0])
	}
}

func TestParseZeroArgumentCallUnchanged(t *testing.T) {
	// Semantic neutrality (CONTRACTS §2): the story 05 zero-arg forms parse
	// exactly as before, with no arguments.
	program := parseSource(t, "clear()\n")
	stmt := program.Statements[0]
	if stmt.Kind != Call || len(stmt.Values) != 0 || stmt.Call.Receiver != "clear" {
		t.Fatalf("statement = %#v, want the unchanged zero-arg call", stmt)
	}
}

func TestParseRejectsReturnValue(t *testing.T) {
	perr := parseReject(t, "return x\n")
	if perr.Category != UnsupportedSyntax || perr.Code != "ANUY1001" {
		t.Fatalf("error = (%s, %s), want UnsupportedSyntax (ANUY1001)", perr.Category, perr.Code)
	}
}

func TestParseRejectsUnclosedNonClosureArguments(t *testing.T) {
	perr := parseReject(t, "f(1,\n2)\n")
	if perr.Category != UnsupportedSyntax {
		t.Fatalf("error = %s, want UnsupportedSyntax", perr.Category)
	}
}

func TestParseRejectsTrailingCommaArgument(t *testing.T) {
	perr := parseReject(t, "f(1,)\n")
	if perr.Category != UnsupportedSyntax {
		t.Fatalf("error = %s, want UnsupportedSyntax", perr.Category)
	}
}

func TestParseRejectsSafeCallStatement(t *testing.T) {
	// Story 08 Q3-A (RFC-002 §33/§42) flips the story 05 deferred-reject:
	// the safe-call statement parses; ordinary-after-safe stays rejected
	// (§49). The acceptance path is pinned in TestParseSafeCallStatement.
	_, err := Parse("user?.save.address()")
	if err == nil {
		t.Fatal("ordinary selector after safe selector accepted in a call statement")
	}
	requireCategory(t, err, UnsupportedSyntax)
}

func TestParseRejectsFunctionDeclarationWithoutParameterList(t *testing.T) {
	perr := parseReject(t, "func f\n")
	if perr.Category != UnsupportedSyntax || perr.Severity != semantic.SeverityError {
		t.Fatalf("error = (%s, %s), want UnsupportedSyntax Error", perr.Category, perr.Severity)
	}
}

func TestParseFunctionDeclarationPureAnnotation(t *testing.T) {
	program := parseSource(t, "//anuy:pure\nfunc noop() {\n}\n")
	if !program.Statements[0].Pure {
		t.Fatalf("statement = %#v, want Pure for the annotated declaration", program.Statements[0])
	}
	plain := parseSource(t, "func noop() {\n}\n")
	if plain.Statements[0].Pure {
		t.Fatalf("statement = %#v, want no Pure without the annotation", plain.Statements[0])
	}
}
