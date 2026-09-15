package semantic

import "testing"

// Story 05 (CONTRACTS §1): non-nil narrowing is a second fact dimension in
// the same fixpoint traversal. RFC-002 §22: a predicate assumption
// establishes the fact; §25: assignment invalidates it; joins intersect.

func TestNarrowEstablishesNonNilForDeref(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 0 {
		t.Fatalf("narrowed deref = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestDerefWithoutNonNilReports(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != UnsafeMemberAccess {
		t.Fatalf("unproven deref = %#v, want one UnsafeMemberAccess", result.Diagnostics)
	}
}

func TestDerefReportsOncePerBlock(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Deref(3), Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("repeated deref = %#v, want one diagnostic per block", result.Diagnostics)
	}
}

func TestAssignmentInvalidatesNonNil(t *testing.T) {
	// RFC-002 §25: assignment invalidates the narrowing (conservative: the
	// experimental layer has no RHS types, so it never re-establishes).
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Assign(3), Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != UnsafeMemberAccess {
		t.Fatalf("deref after assignment = %#v, want one UnsafeMemberAccess", result.Diagnostics)
	}
}

func TestDeclareInvalidatesNonNil(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Declare(3), Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("deref after declare = %#v, want one diagnostic", result.Diagnostics)
	}
}

func TestJoinIntersectsNonNilFacts(t *testing.T) {
	// Only one branch narrows: the fact must not survive the join.
	joined := NewCFG(
		Block{ID: 1},
		Block{ID: 2, Operations: []Operation{Assume(3)}},
		Block{ID: 3},
		Block{ID: 4, Operations: []Operation{Deref(3)}},
	)
	joined.AddEdge(1, 2)
	joined.AddEdge(1, 3)
	joined.AddEdge(2, 4)
	joined.AddEdge(3, 4)
	if got := (Analyzer{}).Analyze(joined); len(got.Diagnostics) != 1 {
		t.Fatalf("one-sided narrowing = %#v, want one diagnostic at the join", got.Diagnostics)
	}
	both := NewCFG(
		Block{ID: 1},
		Block{ID: 2, Operations: []Operation{Assume(3)}},
		Block{ID: 3, Operations: []Operation{Assume(3)}},
		Block{ID: 4, Operations: []Operation{Deref(3)}},
	)
	both.AddEdge(1, 2)
	both.AddEdge(1, 3)
	both.AddEdge(2, 4)
	both.AddEdge(3, 4)
	if got := (Analyzer{}).Analyze(both); len(got.Diagnostics) != 0 {
		t.Fatalf("two-sided narrowing = %#v, want no diagnostics", got.Diagnostics)
	}
}

func TestNonNilIsIndependentOfInitialization(t *testing.T) {
	// Non-nil does not prove initialization...
	nonNil := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Read(3)}},
	)
	if got := (Analyzer{}).Analyze(nonNil); len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != ReadBeforeInitialization {
		t.Fatalf("non-nil without init = %#v, want ReadBeforeInitialization", got.Diagnostics)
	}
	// ...and initialization does not prove non-nil.
	initialized := NewCFG(
		Block{ID: 1, Operations: []Operation{Declare(3), Assign(3), Deref(3)}},
	)
	if got := (Analyzer{}).Analyze(initialized); len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != UnsafeMemberAccess {
		t.Fatalf("init without non-nil = %#v, want UnsafeMemberAccess", got.Diagnostics)
	}
}

func TestNarrowConvergesWithLoopBackEdge(t *testing.T) {
	// The narrowing fixpoint must converge on loop back edges (F-13 guard:
	// every run uses -timeout 30s).
	cfg := NewCFG(
		Block{ID: 1},
		Block{ID: 2, Operations: []Operation{Assume(3)}},
		Block{ID: 3, Operations: []Operation{Deref(3), Read(3)}},
	)
	cfg.AddEdge(1, 2)
	cfg.AddEdge(2, 2)
	cfg.AddEdge(2, 3)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != ReadBeforeInitialization {
		t.Fatalf("loop narrowing = %#v, want only ReadBeforeInitialization", result.Diagnostics)
	}
}
