package parser

import (
	"testing"
)

// Story 82 (RFC-018 §6.29 v3): the constant initializer accepts constant
// references - resolution is the checker's job; the parser only rejects
// non-constant constructs.

func TestParseConstantReference(t *testing.T) {
	program, err := Parse("const A = 5\nconst B = A + 1\n")
	if err != nil {
		t.Fatal(err)
	}
	b := program.Statements[1]
	if b.Kind != Const || len(b.Values) != 1 || b.Values[0].Text != "A + 1" {
		t.Fatalf("B = %+v, want the verbatim reference expression", b)
	}
	if len(b.Values[0].Idents) != 1 || b.Values[0].Idents[0] != "A" {
		t.Fatalf("idents = %v, want [A]", b.Values[0].Idents)
	}
}

func TestParseConstantDivisionRemainder(t *testing.T) {
	for _, source := range []string{"const H = 1 / 2\n", "const Q = 10 % 3\n"} {
		if _, err := Parse(source); err != nil {
			t.Fatalf("Parse(%q) = %v, want the arithmetic form", source, err)
		}
	}
}

// The inline comment runs to the end of the line - the tokenizer stops
// at `//`.
func TestParseInlineComment(t *testing.T) {
	program, err := Parse("const A = 5 // the maximum\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 1 || program.Statements[0].Kind != Const {
		t.Fatalf("statements = %+v, want the constant", program.Statements)
	}
}

// Non-constant constructs keep rejecting at the parser - resolution of
// names does not legalize calls, composites or nil.
func TestParseConstantNonConstConstructsReject(t *testing.T) {
	for _, source := range []string{
		"const x = f()",
		"const x = T{}",
		"const x = nil",
		"const x = x?.y",
	} {
		if _, err := Parse(source); err == nil {
			t.Fatalf("Parse(%q) accepted a non-constant initializer", source)
		}
	}
}
