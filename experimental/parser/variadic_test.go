package parser

import (
	"errors"
	"testing"
)

// Story 71 (RFC-019 §6.14): variadic functions and function types - the
// final parameter spells `name ...T`, the body type is `[]T`, and
// variadicness is part of function type identity.

func TestVariadicParamParses(t *testing.T) {
	program, err := Parse("package p\nfunc Sum(values ...int) int {\nreturn 0\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	param := program.Statements[0].Closure.Params[0]
	if !param.Variadic {
		t.Fatalf("param = %+v, want variadic", param)
	}
	if param.TypeExpr == nil || param.TypeExpr.Name != "int" {
		t.Fatalf("param type = %+v, want int", param.TypeExpr)
	}
}

func TestVariadicClosureParamParses(t *testing.T) {
	program, err := Parse("package p\nvar sum = func(values ...int) int {\nreturn 0\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	param := program.Statements[0].Values[0].Closure.Params[0]
	if !param.Variadic {
		t.Fatalf("param = %+v, want variadic", param)
	}
}

func TestVariadicNonFinalRejects(t *testing.T) {
	_, err := Parse("package p\nfunc Bad(values ...int, other int) int {\nreturn 0\n}\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for a non-final variadic parameter", err)
	}
}

func TestVariadicTypeUnnamedParses(t *testing.T) {
	program, err := Parse("package p\nvar f func(...int) int\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType || len(typ.Params) != 1 || !typ.Params[0].Variadic {
		t.Fatalf("type = %+v, want one variadic parameter", typ)
	}
}

func TestVariadicTypeNamedParses(t *testing.T) {
	program, err := Parse("package p\nvar f func(values ...int) int\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType || len(typ.Params) != 1 {
		t.Fatalf("type = %+v, want one parameter", typ)
	}
	if !typ.Params[0].Variadic || typ.Params[0].Name != "values" {
		t.Fatalf("param = %+v, want named variadic", typ.Params[0])
	}
}

func TestVariadicTypeNonFinalRejects(t *testing.T) {
	_, err := Parse("package p\nvar f func(...int, int) int\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for a non-final variadic parameter", err)
	}
}

func TestVariadicnessSplitsCanonicalSignatures(t *testing.T) {
	program, err := Parse("package p\nvar a func(...int) int\nvar b func([]int) int\n")
	if err != nil {
		t.Fatal(err)
	}
	a := program.Statements[0].TypeExpr.Canonical()
	b := program.Statements[1].TypeExpr.Canonical()
	if a == b {
		t.Fatalf("canonical %q identifies variadic and slice parameters", a)
	}
	if a != "func(...int) int" {
		t.Fatalf("canonical = %q, want func(...int) int", a)
	}
}

func TestVariadicSpreadCallParses(t *testing.T) {
	program, err := Parse("package p\nfunc Sum(values ...int) int {\nreturn 0\n}\nvar v []int\nSum(v...)\n")
	if err != nil {
		t.Fatal(err)
	}
	call := program.Statements[2]
	if call.Kind != Call || len(call.Values) != 1 {
		t.Fatalf("statement = %+v, want one spread argument", call)
	}
}

func TestSpreadNonFinalRejects(t *testing.T) {
	_, err := Parse("package p\nfunc Sum(values ...int) int {\nreturn 0\n}\nvar v []int\nSum(v..., 1)\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for a non-final spread argument", err)
	}
}
