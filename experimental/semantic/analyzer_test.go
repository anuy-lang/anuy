package semantic

import "testing"

func TestAnalyzeAcceptsReadAfterBothBranchAssignments(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1, Operations: []Operation{Declare(1)}},
		Block{ID: 2, Operations: []Operation{Assign(1)}},
		Block{ID: 3, Operations: []Operation{Assign(1)}},
		Block{ID: 4, Operations: []Operation{Read(1)}},
	)
	cfg.AddEdge(1, 2)
	cfg.AddEdge(1, 3)
	cfg.AddEdge(2, 4)
	cfg.AddEdge(3, 4)
	if got := (Analyzer{}).Analyze(cfg).Diagnostics; len(got) != 0 {
		t.Fatalf("diagnostics = %#v, want none", got)
	}
}

func TestAnalyzeRejectsReadWhenBranchMissesAssignment(t *testing.T) {
	cfg := NewCFG(Block{ID: 1, Operations: []Operation{Declare(1)}}, Block{ID: 2, Operations: []Operation{Assign(1)}}, Block{ID: 3}, Block{ID: 4, Operations: []Operation{Read(1)}})
	cfg.AddEdge(1, 2)
	cfg.AddEdge(1, 3)
	cfg.AddEdge(2, 4)
	cfg.AddEdge(3, 4)
	got := (Analyzer{}).Analyze(cfg).Diagnostics
	if len(got) != 1 || got[0].Category != ReadBeforeInitialization {
		t.Fatalf("diagnostics = %#v, want ReadBeforeInitialization", got)
	}
}
