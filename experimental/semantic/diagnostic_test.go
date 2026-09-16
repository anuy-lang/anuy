package semantic

import "testing"

func TestDiagnosticRetainsPrimarySourceSpan(t *testing.T) {
	span := SourceSpan{File: "case.anuy", Start: 3, End: 4}
	got := NewDiagnostic(UnknownAssignmentDescriptor, 1, span)
	if got.Category != UnknownAssignment || got.Span != span {
		t.Fatalf("diagnostic = %#v", got)
	}
}

func TestRefinementDoesNotLeakToShadowedBinding(t *testing.T) {
	r := NewRefinements()
	r.MarkNonNil(1)
	if r.IsNonNil(2) {
		t.Fatal("outer refinement leaked to shadowed BindingID")
	}
}
