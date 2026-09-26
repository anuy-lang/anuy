package parser

import (
	"testing"
)

// Story 76 (RFC-005 §6.7.1, RFC-019 §6.6): direct tail forwarding - a
// single call-shaped return forwards the callee's whole fallible result
// in a compatible fallible frame. `return try` is unnecessary (§6.7.2).

func TestFallibleForwardingParses(t *testing.T) {
	program, err := Parse("package p\nfunc Inner() (int, error?) {\nreturn 1, nil\n}\nfunc Outer() (int, error?) {\nreturn Inner()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	ret := program.Statements[1].Closure.Body[0]
	if ret.Kind != Return || len(ret.Values) != 1 {
		t.Fatalf("return = %+v, want one forwarded value", ret)
	}
}
