package parser

import "testing"

// Story 50 (RFC-011 §6.2.18 precision ladder): the AST carries value
// spans and navigation receiver spans, so read diagnostics can blame the
// exact identifier instead of the enclosing line.
func TestValueSpanCoversConstruct(t *testing.T) {
	program, err := Parse("var b = a.count\n")
	if err != nil {
		t.Fatal(err)
	}
	value := program.Statements[0].Values[0]
	if value.Span != (Span{Start: 8, End: 15}) {
		t.Fatalf("value span = %v, want 8..15", value.Span)
	}
}

func TestNavigationReceiverSpan(t *testing.T) {
	program, err := Parse("user.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	call := program.Statements[0].Call
	if call == nil {
		t.Fatal("call statement expected")
	}
	if call.ReceiverSpan != (Span{Start: 0, End: 4}) {
		t.Fatalf("receiver span = %v, want 0..4", call.ReceiverSpan)
	}
}
