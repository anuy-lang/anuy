package parser

import (
	"errors"
	"testing"
)

// Story 67 (RFC-015 §6.15 v6): qualified type names - `pkg.Type` in type
// positions, verbatim NamedType with the qualifier; the primary motive is
// the Go-compatible test declaration `func TestX(t *testing.T)`
// (§6.5.4 RFC-010).

func TestQualifiedPointerTypeParamParses(t *testing.T) {
	program, err := Parse("package p\nfunc TestX(t *testing.T) {\nreturn\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	param := program.Statements[0].Closure.Params[0]
	if param.TypeExpr == nil || param.TypeExpr.Kind != PointerType {
		t.Fatalf("param type = %+v, want *testing.T", param.TypeExpr)
	}
	if param.TypeExpr.Elem == nil || param.TypeExpr.Elem.Name != "testing.T" {
		t.Fatalf("elem = %+v, want the qualified name testing.T", param.TypeExpr.Elem)
	}
}

func TestQualifiedNamedTypeVarParses(t *testing.T) {
	program, err := Parse("package p\nvar r testing.T\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != NamedType || typ.Name != "testing.T" {
		t.Fatalf("type = %+v, want the qualified named type testing.T", typ)
	}
}

func TestQualifiedTypeMalformedRejects(t *testing.T) {
	for _, source := range []string{
		"package p\nfunc f(t .T) {\n}\n",   // no qualifier
		"package p\nfunc f(t pkg.) {\n}\n", // no name after the dot
	} {
		_, err := Parse(source)
		if err == nil {
			t.Fatalf("Parse(%q) accepted a malformed qualified type", source)
		}
		var perr *Error
		if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
			t.Fatalf("Parse(%q) = %v, want ANUY1001", source, err)
		}
	}
}
