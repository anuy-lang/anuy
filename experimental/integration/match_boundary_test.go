package integration

import "testing"

// Story 51 (RFC-006 §6.6 boundary, ADR-0012 F-46-3): the match
// integration pins — §6.6.1–6.6.4 hold with the accumulated machinery;
// they are the evidence lines for the acceptance map.

// §6.6.1: every reachable arm initializes the binding — the join is
// definitely initialized.
func TestMatchInitializationThroughExhaustiveArms(t *testing.T) {
	source := "type Color enum {\nRed\nGreen\nBlue\n}\nfunc run() {\nvar text string\nvar c Color = Color.Red\nswitch c {\ncase Color.Red:\ntext = \"red\"\ncase Color.Green:\ntext = \"green\"\ncase Color.Blue:\ntext = \"blue\"\n}\ntext\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.6.2: one arm misses the initialization — the read after the match
// reports ReadBeforeInitialization.
func TestMatchMissingInitializationReports(t *testing.T) {
	source := "type Color enum {\nRed\nGreen\n}\nfunc run() {\nvar text string\nvar c Color = Color.Red\nswitch c {\ncase Color.Red:\ntext = \"red\"\ncase Color.Green:\n}\ntext\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY3001" {
		t.Fatalf("diagnostics = %#v, want one ANUY3001", result.Diagnostics)
	}
}

// §6.6.3: a diverging arm (`return error …`) does not require the
// initialization — ordinary dataflow rules apply to the match CFG.
func TestMatchDivergingArmDoesNotRequireInitialization(t *testing.T) {
	source := "type State enum {\nReady\nInvalid\n}\nfunc compute() int {\nreturn 1\n}\nfunc invalidState() error {\nreturn nil\n}\nfunc run() error? {\nvar value int\nvar st State = State.Ready\nswitch st {\ncase State.Ready:\nvalue = compute()\ncase State.Invalid:\nreturn error invalidState()\n}\nvalue\nreturn nil\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}

// §6.6.4: try inside an arm propagates from the enclosing function, not
// from the match.
func TestMatchArmTryDeclaresAndPropagates(t *testing.T) {
	source := "type Source enum {\nLocal\nRemote\n}\nfunc load() (int, error?) {\nreturn 1, nil\n}\nfunc run() error? {\nvar s Source = Source.Local\nswitch s {\ncase Source.Local:\nvar data = try load()\ndata\ncase Source.Remote:\nvar d2 = try load()\nd2\n}\nreturn nil\n}\n"
	result, err := AnalyzeSource(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %#v, want none", result.Diagnostics)
	}
}
