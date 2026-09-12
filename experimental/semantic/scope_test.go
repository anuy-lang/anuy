package semantic

import "testing"

func TestScopeRejectsRedeclarationAndUnknownAssignment(t *testing.T) {
	s := NewScope()
	if err := s.Declare("x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Declare("x"); err == nil || err.Category != SameScopeRedeclaration {
		t.Fatalf("Declare duplicate = %#v", err)
	}
	if err := s.Assign("missing"); err == nil || err.Category != UnknownAssignment {
		t.Fatalf("Assign missing = %#v", err)
	}
}

func TestScopeAssignmentTargetsDeclaredBinding(t *testing.T) {
	s := NewScope()
	_ = s.Declare("x")
	if err := s.Assign("x"); err != nil {
		t.Fatal(err)
	}
}
