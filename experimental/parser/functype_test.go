package parser

import (
	"errors"
	"testing"
)

// Story 69 (RFC-019 §6.2 v2): function types in type positions - the
// F-68-4 grammar gap. Parameter names are documentation and do not
// affect identity; the result is a single type or a parenthesized list.

func TestFuncTypeParamParses(t *testing.T) {
	program, err := Parse("package p\nfunc OnRequest(cb func(c Color) int) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	param := program.Statements[0].Closure.Params[0]
	typ := param.TypeExpr
	if typ == nil || typ.Kind != FuncType {
		t.Fatalf("param type = %+v, want a function type", typ)
	}
	if len(typ.Params) != 1 || typ.Params[0].Name != "c" {
		t.Fatalf("func type params = %+v, want one named c", typ.Params)
	}
	if typ.Params[0].Type == nil || typ.Params[0].Type.Name != "Color" {
		t.Fatalf("func type param type = %+v, want Color", typ.Params[0].Type)
	}
	if len(typ.Results) != 1 || typ.Results[0].Name != "int" {
		t.Fatalf("func type results = %+v, want int", typ.Results)
	}
}

func TestFuncTypeUnnamedParamsAndResultListParse(t *testing.T) {
	program, err := Parse("package p\nvar h func(string, int) (string, error?)\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType {
		t.Fatalf("type = %+v, want a function type", typ)
	}
	if len(typ.Params) != 2 || typ.Params[0].Name != "" || typ.Params[1].Name != "" {
		t.Fatalf("params = %+v, want two unnamed", typ.Params)
	}
	if len(typ.Results) != 2 || !typ.Results[1].Nullable || typ.Results[1].Name != "error" {
		t.Fatalf("results = %+v, want (string, error?)", typ.Results)
	}
}

func TestFuncTypeNoResultParses(t *testing.T) {
	program, err := Parse("package p\nvar onDone func()\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType || len(typ.Results) != 0 {
		t.Fatalf("type = %+v, want a result-less function type", typ)
	}
}

func TestFuncTypeNestsInComposites(t *testing.T) {
	for _, source := range []string{
		"package p\nvar hs []func(Event)\n",
		"package p\nvar m map[string]func() error?\n",
		"package p\nvar p *func()\n",
		"package p\nvar outer func(cb func(int) int) func() int\n",
	} {
		program, err := Parse(source)
		if err != nil {
			t.Fatalf("Parse(%q): %v", source, err)
		}
		typ := program.Statements[0].TypeExpr
		if typ == nil || !containsFuncType(typ) {
			t.Fatalf("Parse(%q) = %+v, want a nested function type", source, typ)
		}
	}
}

func containsFuncType(t *TypeExpr) bool {
	if t == nil {
		return false
	}
	if t.Kind == FuncType {
		return true
	}
	if containsFuncType(t.Elem) || containsFuncType(t.Key) || containsFuncType(t.Value) {
		return true
	}
	for _, p := range t.Params {
		if containsFuncType(p.Type) {
			return true
		}
	}
	for _, r := range t.Results {
		if containsFuncType(r) {
			return true
		}
	}
	return false
}

func TestFuncTypeMixedParamsReject(t *testing.T) {
	// Grouped spellings `a, b int` resolve per name group in Go; the
	// per-parameter reading of this slice would misbind them, so the
	// mixed shape rejects (story 69 non-goal).
	_, err := Parse("package p\nvar bad func(a, b int) int\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for mixed named and unnamed parameters", err)
	}
}

func TestFuncTypeNamedResultsReject(t *testing.T) {
	_, err := Parse("package p\nvar bad func() (v int)\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for named results in a function type", err)
	}
}

// Story 69 (RFC-019 §6.3 v2): nullability suffix binding for function
// types - the trailing `?` after a single result binds to the result;
// the nullable whole function parenthesizes.
func TestNullableFuncTypeParses(t *testing.T) {
	program, err := Parse("package p\nvar cb (func(int) int)?\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType || !typ.Nullable {
		t.Fatalf("type = %+v, want a nullable function type", typ)
	}
}

func TestNullableResultFuncTypeParses(t *testing.T) {
	program, err := Parse("package p\nvar f func() int?\n")
	if err != nil {
		t.Fatal(err)
	}
	typ := program.Statements[0].TypeExpr
	if typ == nil || typ.Kind != FuncType || typ.Nullable {
		t.Fatalf("type = %+v, want a non-null function returning int?", typ)
	}
	if len(typ.Results) != 1 || !typ.Results[0].Nullable {
		t.Fatalf("results = %+v, want one nullable int", typ.Results)
	}
}

func TestAmbiguousNullableResultListRejects(t *testing.T) {
	_, err := Parse("package p\nvar bad func() (int, error?)?\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for ? after a result list", err)
	}
}
