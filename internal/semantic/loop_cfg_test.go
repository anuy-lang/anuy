package semantic

import "testing"

// Loop CFG shape: entry 1 declares the binding, header 2 evaluates the
// condition, body 3 runs the loop operations, exit 4 observes the binding
// after the loop. Edges follow RFC-003 §70–74.
func loopCFG(entry, header, body, exit []Operation) *CFG {
	cfg := NewCFG(
		Block{ID: 1, Operations: entry},
		Block{ID: 2, Operations: header},
		Block{ID: 3, Operations: body},
		Block{ID: 4, Operations: exit},
	)
	cfg.AddEdge(1, 2)
	cfg.AddEdge(2, 3)
	cfg.AddEdge(3, 2)
	return cfg
}

func requireSingleReadBeforeInit(t *testing.T, result AnalysisResult) {
	t.Helper()
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != ReadBeforeInitialization {
		t.Fatalf("result = %#v, want one ReadBeforeInitialization", result)
	}
}

func requireNoDiagnostics(t *testing.T, result AnalysisResult) {
	t.Helper()
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestLoopJoinKeepsZeroIterationPathUninitialized(t *testing.T) {
	cfg := loopCFG(
		[]Operation{Declare(1)},
		nil,
		[]Operation{Assign(1)},
		[]Operation{Read(1)},
	)
	cfg.AddEdge(2, 4)
	result := (Analyzer{}).Analyze(cfg)
	requireSingleReadBeforeInit(t, result)
}

func TestLoopJoinAcceptsInitializationBeforeLoop(t *testing.T) {
	cfg := loopCFG(
		[]Operation{Declare(1), Assign(1)},
		nil,
		[]Operation{Assign(1)},
		[]Operation{Read(1)},
	)
	cfg.AddEdge(2, 4)
	result := (Analyzer{}).Analyze(cfg)
	requireNoDiagnostics(t, result)
}

func TestLoopHeaderReadRequiresInitialization(t *testing.T) {
	cfg := loopCFG(
		[]Operation{Declare(1)},
		[]Operation{Read(1)},
		[]Operation{Assign(1)},
		nil,
	)
	result := (Analyzer{}).Analyze(cfg)
	requireSingleReadBeforeInit(t, result)
}

func TestLoopBodyReadAfterAssignmentIsSafe(t *testing.T) {
	cfg := loopCFG(
		[]Operation{Declare(1)},
		nil,
		[]Operation{Assign(1), Read(1)},
		nil,
	)
	result := (Analyzer{}).Analyze(cfg)
	requireNoDiagnostics(t, result)
}

func TestInfiniteLoopAcceptsExitWhereEveryPathAssigns(t *testing.T) {
	// RFC-003 §72: the only reachable exit is taken after the body assigned
	// the binding, so the exit may observe it.
	cfg := loopCFG(
		[]Operation{Declare(1)},
		nil,
		[]Operation{Assign(1)},
		[]Operation{Read(1)},
	)
	cfg.AddEdge(3, 4)
	result := (Analyzer{}).Analyze(cfg)
	requireNoDiagnostics(t, result)
}

func TestInfiniteLoopRejectsExitReachableWithoutAssignment(t *testing.T) {
	// RFC-003 §73: one reachable loop exit skips the assignment, so the exit
	// cannot prove initialization.
	cfg := loopCFG(
		[]Operation{Declare(1)},
		nil,
		[]Operation{Assign(1)},
		[]Operation{Read(1)},
	)
	cfg.AddEdge(3, 4)
	cfg.AddEdge(2, 4)
	result := (Analyzer{}).Analyze(cfg)
	requireSingleReadBeforeInit(t, result)
}
