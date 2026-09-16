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

func TestCallInvalidatesNonNilOfMutatedBindings(t *testing.T) {
	// RFC-003 §85: calling a closure that assigns a captured binding
	// invalidates that binding's non-nil narrowing; unrelated bindings and
	// read-only calls leave it untouched.
	withMutation := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Call(3), Deref(3)}},
	)
	if got := (Analyzer{}).Analyze(withMutation); len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != UnsafeMemberAccess {
		t.Fatalf("mutating call = %#v, want UnsafeMemberAccess", got.Diagnostics)
	}
	withoutMutation := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Call(), Call(4), Deref(3)}},
	)
	if got := (Analyzer{}).Analyze(withoutMutation); len(got.Diagnostics) != 0 {
		t.Fatalf("read-only call = %#v, want no diagnostics", got.Diagnostics)
	}
}

func TestCallDoesNotProveInitialization(t *testing.T) {
	// RFC-003 §84: a closure call never establishes caller-local
	// initialization - the call operation touches only the non-nil
	// dimension.
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Assume(3), Call(3), Read(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != ReadBeforeInitialization {
		t.Fatalf("call then read = %#v, want ReadBeforeInitialization", result.Diagnostics)
	}
}

func TestCapturedNonNilSeedingPattern(t *testing.T) {
	// CONTRACTS §1.5: the closure body starts from the creation-point
	// non-nil fact; the integration emits the entry ops, the kernel proves
	// them.
	seeded := NewCFG(
		Block{ID: 1, Operations: []Operation{Declare(3), Assign(3), Assume(3), Deref(3)}},
	)
	if got := (Analyzer{}).Analyze(seeded); len(got.Diagnostics) != 0 {
		t.Fatalf("seeded capture = %#v, want no diagnostics", got.Diagnostics)
	}
	unseeded := NewCFG(
		Block{ID: 1, Operations: []Operation{Declare(3), Assign(3), Deref(3)}},
	)
	if got := (Analyzer{}).Analyze(unseeded); len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != UnsafeMemberAccess {
		t.Fatalf("unseeded capture = %#v, want UnsafeMemberAccess", got.Diagnostics)
	}
}

func TestCascadeDerefFoldsIntoReadPrimary(t *testing.T) {
	// Решение 4 / CONTRACTS §2: one block, one binding, both dimensions —
	// the deref fires before the read yet nests into the
	// ReadBeforeInitialization primary and is not published standalone.
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Deref(3), Read(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("cascade = %#v, want one primary diagnostic", result.Diagnostics)
	}
	primary := result.Diagnostics[0]
	if primary.Category != ReadBeforeInitialization || primary.Code != "ANUY3001" {
		t.Fatalf("primary = (%s, %s), want ReadBeforeInitialization (ANUY3001)", primary.Category, primary.Code)
	}
	if len(primary.Related) != 1 || primary.Related[0].Category != UnsafeMemberAccess || primary.Related[0].Code != "ANUY4001" {
		t.Fatalf("related = %#v, want one UnsafeMemberAccess (ANUY4001)", primary.Related)
	}
}

func TestCascadeReadPrimaryAttachesLaterDeref(t *testing.T) {
	// CONTRACTS §2: the read-first order yields the same primary+related
	// shape without a standalone duplicate.
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Read(3), Deref(3)}},
	)
	result := (Analyzer{}).Analyze(cfg)
	if len(result.Diagnostics) != 1 {
		t.Fatalf("cascade = %#v, want one primary diagnostic", result.Diagnostics)
	}
	primary := result.Diagnostics[0]
	if primary.Category != ReadBeforeInitialization {
		t.Fatalf("primary = %s, want ReadBeforeInitialization", primary.Category)
	}
	if len(primary.Related) != 1 || primary.Related[0].Category != UnsafeMemberAccess {
		t.Fatalf("related = %#v, want one UnsafeMemberAccess", primary.Related)
	}
}

func TestSoloDerefAndReadStayStandalone(t *testing.T) {
	// CONTRACTS §2.3: outside the cascade both rules publish independently,
	// with no related information.
	soloDeref := NewCFG(
		Block{ID: 1, Operations: []Operation{Deref(3)}},
	)
	got := (Analyzer{}).Analyze(soloDeref)
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != UnsafeMemberAccess || len(got.Diagnostics[0].Related) != 0 {
		t.Fatalf("solo deref = %#v, want one standalone UnsafeMemberAccess", got.Diagnostics)
	}
	soloRead := NewCFG(
		Block{ID: 1, Operations: []Operation{Read(3)}},
	)
	got = (Analyzer{}).Analyze(soloRead)
	if len(got.Diagnostics) != 1 || got.Diagnostics[0].Category != ReadBeforeInitialization || len(got.Diagnostics[0].Related) != 0 {
		t.Fatalf("solo read = %#v, want one standalone ReadBeforeInitialization", got.Diagnostics)
	}
}

func TestCascadeDoesNotCrossBlocks(t *testing.T) {
	// CONTRACTS §2.1: the cascade is per (block, binding) - a read primary
	// in one block and a deref in another stay standalone.
	cfg := NewCFG(
		Block{ID: 1},
		Block{ID: 2, Operations: []Operation{Read(3)}},
		Block{ID: 3, Operations: []Operation{Deref(3)}},
	)
	cfg.AddEdge(1, 2)
	cfg.AddEdge(1, 3)
	got := (Analyzer{}).Analyze(cfg)
	if len(got.Diagnostics) != 2 {
		t.Fatalf("cross-block = %#v, want two standalone diagnostics", got.Diagnostics)
	}
	for _, d := range got.Diagnostics {
		if len(d.Related) != 0 {
			t.Fatalf("%s carries related %#v, want none across blocks", d.Category, d.Related)
		}
	}
}
