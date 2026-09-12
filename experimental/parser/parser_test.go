package parser

import "testing"

func TestParseVarAndAssignmentWithSpans(t *testing.T) {
	program, err := Parse("var x\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(program.Statements) != 2 || program.Statements[0].Kind != Var || program.Statements[1].Kind != Assign {
		t.Fatalf("program = %#v", program)
	}
	if program.Statements[1].Span.Start != 6 {
		t.Fatalf("assignment span = %#v", program.Statements[1].Span)
	}
}

func TestParseRejectsUnsupportedSyntax(t *testing.T) {
	if _, err := Parse("func main() {}\n"); err == nil {
		t.Fatal("unsupported syntax accepted")
	}
}
