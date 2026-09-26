package integration

import (
	"testing"
)

// Story 73 (RFC-019 §6.7, F-70-3): the §6.7 narrowing rule covers every
// invocation of a nullable function value - the value positions
// (initializer, assignment, return, argument) gate like the statement
// form (ANUY4011, story 70).

func TestValuePositionInitializerGateReports(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = nil\nvar x = cb()\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableFunctionCall")
}

func TestValuePositionAssignGateReports(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = nil\nvar x int\nx = cb()\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableFunctionCall")
}

func TestValuePositionReturnGateReports(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = nil\nfunc Run() int {\nreturn cb()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableFunctionCall")
}

func TestValuePositionArgumentGateReports(t *testing.T) {
	// The statement-call argument is a value position (§6.7). A nested
	// call inside an initializer (`var x = Apply(cb())`) stays with the
	// outermost-callee boundary (story 73 non-goal).
	result, err := AnalyzeSource("var cb (func() int)? = nil\nfunc Apply(f func() int) int {\nreturn f()\n}\nApply(cb())\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "NullableFunctionCall")
}

func TestNarrowedValuePositionsClean(t *testing.T) {
	result, err := AnalyzeSource("var cb (func() int)? = nil\nfunc Apply(f func() int) int {\nreturn f()\n}\nvar a int = 0\nvar b int = 0\nif cb != nil {\na = cb()\nb = Apply(cb())\n}\na\nb\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none (narrowed)", result.Diagnostics)
	}
}

func TestMethodCallValueNotGated(t *testing.T) {
	// A dotted callee is the method-call world: the receiver carries the
	// deref gate (ANUY4001), the nullable-call gate never fires for it.
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc (c Counter) Value() int {\nreturn c.n\n}\nfunc Find() Counter? {\nreturn Counter{n: 0}\n}\nvar c Counter? = Find()\nvar x = c.Value()\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range result.Diagnostics {
		if d.Category == "NullableFunctionCall" {
			t.Fatalf("dotted callee hit the nullable-call gate: %v", d)
		}
	}
}

func TestSpreadArgumentNotGated(t *testing.T) {
	// A spread argument is the slice value, not an invocation (§6.14).
	result, err := AnalyzeSource("func Sum(values ...int) int {\nreturn 0\n}\nvar v = Make()\nvar total = Sum(v...)\ntotal\nfunc Make() []int {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range result.Diagnostics {
		if d.Category == "NullableFunctionCall" {
			t.Fatalf("spread argument hit the nullable-call gate: %v", d)
		}
	}
}
