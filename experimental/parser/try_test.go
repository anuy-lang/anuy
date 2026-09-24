package parser

import "testing"

// Story 51 (RFC-006 §6.6.5, RFC-005 §6.5.6/§6.5.7): try is not a value.
// The whole-RHS forms are the only propagation positions, and a hidden
// try inside a value position — including a value-switch arm — is not
// allowed in v1.
func TestTryIsNotAValue(t *testing.T) {
	if _, err := Parse("x = try f()\n"); err == nil {
		t.Fatal("try in an assignment RHS must be rejected")
	}
	if _, err := Parse("var v int = switch s {\ncase A.B:\ntry f()\ncase C.D:\n2\n}\n"); err == nil {
		t.Fatal("try inside a value-switch arm must be rejected")
	}
	if _, err := Parse("var x = transform(try f())\n"); err == nil {
		t.Fatal("nested try inside an expression must be rejected")
	}
}

// Story 51: the recognized whole-RHS try forms keep parsing (stories
// 34/35) — reserving the keyword must not break them.
func TestTryWholeRHSFormsStillParse(t *testing.T) {
	if _, err := Parse("var v = try F(x)\n"); err != nil {
		t.Fatalf("value-try declaration must still parse: %v", err)
	}
	if _, err := Parse("try Save(data)\n"); err != nil {
		t.Fatalf("standalone try must still parse: %v", err)
	}
}
