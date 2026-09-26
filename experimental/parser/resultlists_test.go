package parser

import (
	"errors"
	"testing"
)

// Story 74 (RFC-019 §6.5 v6): result list spellings - `(T)` is the
// parenthesized single result, `(T)?` makes it nullable (required for
// composite result types), a list without trailing `error?` is an
// ordinary non-fallible multi-result, and `?` after a multi-list rejects.

func TestSingleParenResultParses(t *testing.T) {
	program, err := Parse("package p\nfunc Make() (int) {\nreturn 0\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Statements[0]
	if !fn.HasResult || fn.ResultNullable {
		t.Fatalf("result = (%v, %v), want non-nullable single", fn.HasResult, fn.ResultNullable)
	}
	if fn.Closure.ResultTypeExpr == nil || fn.Closure.ResultTypeExpr.Name != "int" {
		t.Fatalf("result type = %+v, want int", fn.Closure.ResultTypeExpr)
	}
}

func TestNullableParenResultParses(t *testing.T) {
	program, err := Parse("package p\nfunc Pair() (int)? {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Statements[0]
	if !fn.HasResult || !fn.ResultNullable {
		t.Fatalf("result = (%v, %v), want nullable single", fn.HasResult, fn.ResultNullable)
	}
}

func TestNullableFuncResultListParses(t *testing.T) {
	// The story-73 blocker: a declared function returning the nullable
	// whole-function type - the parenthesized spelling is required.
	program, err := Parse("package p\nfunc Make() (func() int)? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Statements[0]
	if !fn.HasResult || !fn.ResultNullable {
		t.Fatalf("result = (%v, %v), want nullable single", fn.HasResult, fn.ResultNullable)
	}
	typ := fn.Closure.ResultTypeExpr
	if typ == nil || typ.Kind != FuncType || !typ.Nullable {
		t.Fatalf("result type = %+v, want a nullable function type", typ)
	}
}

func TestPlainMultiResultParses(t *testing.T) {
	// The §6.5 example: a list without trailing `error?` is an ordinary
	// non-fallible multi-result.
	program, err := Parse("package p\nfunc Split(v string) (string, string) {\nreturn v, v\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Statements[0]
	if !fn.HasResult {
		t.Fatalf("HasResult = false, want true")
	}
	if len(fn.Closure.ResultList) != 2 {
		t.Fatalf("result list = %+v, want two results", fn.Closure.ResultList)
	}
}

func TestFallibleListStillParses(t *testing.T) {
	program, err := Parse("package p\nfunc Load() (int, error?) {\nreturn 0, nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	fn := program.Statements[0]
	if len(fn.Closure.ResultList) != 2 {
		t.Fatalf("result list = %+v, want two results", fn.Closure.ResultList)
	}
	last := fn.Closure.ResultList[1]
	if last.Name != "error" || !last.Nullable {
		t.Fatalf("last = %+v, want error?", last)
	}
}

func TestNullableAfterMultiListRejects(t *testing.T) {
	_, err := Parse("package p\nfunc Bad() (int, string)? {\nreturn \"\", \"\"\n}\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001 for ? after a multi-result list", err)
	}
}

func TestPlainMultiReturnArityRejects(t *testing.T) {
	_, err := Parse("package p\nfunc Split(v string) (string, string) {\nreturn v\n}\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1006" {
		t.Fatalf("err = %v, want ANUY1006 for a return-count mismatch", err)
	}
}
