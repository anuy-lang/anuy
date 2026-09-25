package parser

import (
	"errors"
	"testing"
)

// Story 68 (RFC-008 §6.9.10 v4, §6.9.7 RFC-007): a closure literal takes
// a declared result between the parameter list and the block, so a
// result-carrying callback can lower to the validating wrapper with
// result propagation (closes F-55-1).

func TestClosureLiteralSingleResultParses(t *testing.T) {
	program, err := Parse("package p\nfunc run() {\ngoRegister(func(c Color) int {\nreturn 1\n})\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 1 || len(program.Statements[0].Closure.Body) != 1 {
		t.Fatalf("program = %+v, want one function with the call in its body", program)
	}
	call := program.Statements[0].Closure.Body[0]
	if call.Kind != Call || len(call.Values) != 1 || call.Values[0].Closure == nil {
		t.Fatalf("statement = %+v, want a call with a closure argument", call)
	}
	cl := call.Values[0].Closure
	if !cl.HasResult || cl.ResultTypeExpr == nil || cl.ResultTypeExpr.Name != "int" {
		t.Fatalf("closure = %+v, want the declared int result", cl)
	}
}

func TestClosureLiteralResultVarFormParses(t *testing.T) {
	program, err := Parse("package p\nvar f = func(c Color) int {\nreturn 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	cl := program.Statements[0].Values[0].Closure
	if cl == nil || !cl.HasResult || cl.ResultTypeExpr == nil || cl.ResultTypeExpr.Name != "int" {
		t.Fatalf("closure = %+v, want the declared int result", cl)
	}
}

// A non-fallible result list rejects - the slice gate accepts only the
// `(T, error?)` shape (story 35, RFC-005 §6.2.4).
func TestClosureNonFallibleListRejects(t *testing.T) {
	_, err := Parse("package p\nfunc run() {\ngoRegister(func(c Color) (int, bool) {\nreturn 1, true\n})\n}\n")
	if err == nil {
		t.Fatal("Parse accepted a non-fallible result list")
	}
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001", err)
	}
}
