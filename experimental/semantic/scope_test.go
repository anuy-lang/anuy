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

func TestScopeReadReportsUnresolvedName(t *testing.T) {
	s := NewScope()
	if err := s.Read("x"); err == nil || err.Category != UnknownRead {
		t.Fatalf("Read unresolved = %#v, want UnknownRead", err)
	}
	if err := s.Declare("x"); err != nil {
		t.Fatal(err)
	}
	if err := s.Read("x"); err != nil {
		t.Fatalf("Read declared = %#v", err)
	}
	if err := s.Child().Read("x"); err != nil {
		t.Fatalf("Read through parent chain = %#v", err)
	}
}
