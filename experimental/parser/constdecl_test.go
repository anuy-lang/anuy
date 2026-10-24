package parser

import (
	"errors"
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 80 (RFC-018 §6.5/§6.29 v2, RFC-024 §6.23 v4): the package
// constant declaration - a single name with a constant-expression
// initializer the layer lowers verbatim.

func TestParsePackageConstant(t *testing.T) {
	program, err := Parse("const MaxRetries = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 1 {
		t.Fatalf("statements = %d, want one", len(program.Statements))
	}
	statement := program.Statements[0]
	if statement.Kind != Const {
		t.Fatalf("kind = %v, want Const", statement.Kind)
	}
	if len(statement.Names) != 1 || statement.Names[0] != "MaxRetries" {
		t.Fatalf("names = %v, want [MaxRetries]", statement.Names)
	}
	if len(statement.Values) != 1 || statement.Values[0].Text != "5" {
		t.Fatalf("values = %+v, want the raw literal", statement.Values)
	}
}

func TestParseTypedPackageConstant(t *testing.T) {
	program, err := Parse("const Max int = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	statement := program.Statements[0]
	if statement.Type != "int" || statement.TypeExpr == nil {
		t.Fatalf("type = %q (%+v), want the int spelling", statement.Type, statement.TypeExpr)
	}
	if statement.TypeExpr.Kind != NamedType || statement.TypeExpr.Nullable {
		t.Fatalf("type expr = %+v, want a non-nullable named type", statement.TypeExpr)
	}
}

func TestParseConstantBoolLiteral(t *testing.T) {
	program, err := Parse("const OK = true\n")
	if err != nil {
		t.Fatal(err)
	}
	values := program.Statements[0].Values
	if len(values) != 1 || values[0].Text != "true" {
		t.Fatalf("values = %+v, want the raw true literal", values)
	}
}

// The constant expression lowers verbatim - the layer never evaluates it
// (§6.29: the Go toolchain is the evaluator).
func TestParseConstantExpressionVerbatim(t *testing.T) {
	program, err := Parse("const Delta = 1 + 2\n")
	if err != nil {
		t.Fatal(err)
	}
	values := program.Statements[0].Values
	if len(values) != 1 || values[0].Text != "1 + 2" {
		t.Fatalf("values = %+v, want the verbatim expression", values)
	}
}

func rejectConstAssert(t *testing.T, source string, code semantic.Code) {
	t.Helper()
	_, err := Parse(source)
	if err == nil {
		t.Fatalf("Parse(%q) accepted an invalid constant declaration", source)
	}
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != code {
		t.Fatalf("Parse(%q) = %v, want %s", source, err, code)
	}
}

// §6.29 v2 boundaries: the forms outside the minimal slice reject at the
// parser until their slices.
func TestParseConstantFormRejects(t *testing.T) {
	for _, source := range []string{
		"const a, b = 1, 2", // multi-name
		"const ( A = 1 )",   // grouped
		"const x = f()",     // call
		"const x = nil",     // nil is not a constant
		"const x = T{}",     // composite literal
		"const x T? = 5",    // nullable type
		"const p *T = x",    // pointer type
		"const x = 1, 2",    // multi-value initializer
		"const",             // bare keyword
		"var const = 1",     // const leaves the identifier space
	} {
		rejectConstAssert(t, source, "ANUY1001")
	}
}

// §6.5: a constant declaration without an initializer is the bare
// declaration reject (the var analogue).
func TestParseConstantBareRejects(t *testing.T) {
	rejectConstAssert(t, "const Max int", "ANUY1003")
}

// §6.23 RFC-024: the form is package-level in this slice - a body-level
// const rejects.
func TestParseBodyLevelConstantRejects(t *testing.T) {
	rejectConstAssert(t, "func f() {\nconst x = 1\n}", "ANUY1001")
}
