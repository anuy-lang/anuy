package parser

import (
	"testing"
)

// Story 72 (RFC-019 §6.21): the pointer method expression head `(*T).M`
// is a navigation value whose root is the type spelling - a type name is
// not a runtime read.

func TestPointerMethodExpressionHeadParses(t *testing.T) {
	program, err := Parse("package p\ntype Counter struct {\nn int\n}\nvar f = (*Counter).Inc\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[1].Values[0]
	nav := value.Navigation
	if nav == nil {
		t.Fatalf("value = %+v, want a navigation", value)
	}
	if nav.Receiver != "(*Counter)" {
		t.Fatalf("receiver = %q, want the pointer type spelling", nav.Receiver)
	}
	if len(nav.Segments) != 1 || nav.Segments[0].Name != "Inc" {
		t.Fatalf("segments = %+v, want one Inc segment", nav.Segments)
	}
	if len(value.Idents) != 0 {
		t.Fatalf("idents = %v, want none (a type name is not a read)", value.Idents)
	}
}
