package integration

import (
	"strings"
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

func hasCode(result Result, code semantic.Code) bool {
	for _, diag := range result.Diagnostics {
		if diag.Code == code {
			return true
		}
	}
	return false
}

func codes(result Result) string {
	var out []string
	for _, diag := range result.Diagnostics {
		out = append(out, string(diag.Code))
	}
	return strings.Join(out, ",")
}

// RFC-024 §6.1 v4 RFC-015 / §6.10: package variables pre-register into the
// package namespace - a body declared before the variable still resolves
// and reads it initialized (§6.10: variable initialization completes
// before any body executes).
func TestPackageVarBodySeesInitializedBeforeDeclaration(t *testing.T) {
	result, err := AnalyzeSource("func shout() {\nGreeting\n}\nvar Greeting = \"hello\"")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", codes(result))
	}
}

// §6.1 + RFC-002 §6.3.11: the declared `T?` class of a package variable
// participates in narrowing for every body, regardless of declaration
// order - the deref gate fires for a function declared before the
// variable.
func TestPackageVarNullableClassVisibleBeforeDeclaration(t *testing.T) {
	src := "type Cfg struct {\nName string\n}\nfunc probe() string {\nreturn cfg.Name\n}\nvar cfg Cfg? = nil"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY4001") {
		t.Fatalf("diagnostics = %s, want ANUY4001 (deref gate through the pre-registered class)", codes(result))
	}
}

// §8.2: an initialization cycle is a compile-time error.
func TestPackageVarCycleReportsANUY3004(t *testing.T) {
	result, err := AnalyzeSource("var a = b\nvar b = a")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3004") {
		t.Fatalf("diagnostics = %s, want ANUY3004", codes(result))
	}
}

// §6.4/§8.2: dependency closure is transitive through same-package
// function bodies - writes count as references, so a function assigning
// the variable closes a self-cycle.
func TestPackageVarCycleThroughFunctions(t *testing.T) {
	result, err := AnalyzeSource("var x = f()\nfunc f() {\nx = 1\n}")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3004") {
		t.Fatalf("diagnostics = %s, want ANUY3004", codes(result))
	}
}

// §6.4: a forward textual dependency is legal - the dependency graph
// reorders it (§6.2 motivation), no cycle.
func TestPackageVarForwardDependencyLegal(t *testing.T) {
	result, err := AnalyzeSource("var b = a\nvar a = 5")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", codes(result))
	}
}

// §8.3: a call through a func-typed value cannot have its package-state
// dependencies enumerated - the initializer rejects.
func TestPackageVarFuncValueCallRejectsDependency(t *testing.T) {
	result, err := AnalyzeSource("var cb func() = func() {\n}\nvar x = cb()")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3005") {
		t.Fatalf("diagnostics = %s, want ANUY3005", codes(result))
	}
}

// §6.4/§8.3: interface dispatch in an initializer is unknown dynamic
// same-package dispatch - never treated as dependency-free.
func TestPackageVarInterfaceDispatchRejectsDependency(t *testing.T) {
	src := "interface S {\nString() string\n}\ntype implS struct {\n}\nfunc (i implS) String() string {\nreturn \"\"\n}\nimpl S for implS\nvar s S = implS{}\nvar x = s.String()"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3005") {
		t.Fatalf("diagnostics = %s, want ANUY3005", codes(result))
	}
}

// §6.6: a multi-variable group from a declared non-fallible callee with a
// matching result count is one legal initialization step.
func TestPackageVarMultiNameGroupFromDeclaredCallee(t *testing.T) {
	src := "func MakePair() (int, int) {\nreturn 1, 2\n}\nvar a, b = MakePair()"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", codes(result))
	}
}

// §6.3/§8.4: a fallible destructuring cannot declare package variables -
// the conditional shape would leave package state unavailable.
func TestPackageVarFallibleDestructuringRejects(t *testing.T) {
	src := "func Load() (string, error?) {\nreturn \"x\", nil\n}\nvar d, err = Load()\nerr"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3006") {
		t.Fatalf("diagnostics = %s, want ANUY3006", codes(result))
	}
}

// §6.3/§8.4: the comma-ok shape is conditional by construction - the
// presence flag makes the first value unavailable on the failed path -
// regardless of name resolution.
func TestPackageVarCommaOkShapeRejects(t *testing.T) {
	result, err := AnalyzeSource("var u, found = users[id]")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3006") {
		t.Fatalf("diagnostics = %s, want ANUY3006", codes(result))
	}
}

// §6.6: parallel values initialize each binding unconditionally - legal.
func TestPackageVarParallelValuesAccepted(t *testing.T) {
	result, err := AnalyzeSource("var x, y = 1, 2\nx\ny")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", codes(result))
	}
}

// §6.3/§8.4: a comma-ok assignment cannot directly target a package
// variable - extract into a local conditional binding first.
func TestPackageVarCommaOkAssignmentRejects(t *testing.T) {
	result, err := AnalyzeSource("var g = 1\nvar found = false\ng, found = users[id]")
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3006") {
		t.Fatalf("diagnostics = %s, want ANUY3006", codes(result))
	}
}

// §6.3: a parallel assignment carries no conditional shape - ordinary
// reassignment of package state.
func TestPackageVarParallelAssignmentAccepted(t *testing.T) {
	result, err := AnalyzeSource("var x = 1\nvar y = 2\nx, y = y, x\nx\ny")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none", codes(result))
	}
}

// F-64-1 tolerance (§6.22 v2): an unresolved callee in a package
// initializer stays in the tolerance class - in the pure-.anuy layer
// unresolved = undefined, caught by the §6.4.8 go-stage.
func TestPackageVarUnknownCalleeTolerated(t *testing.T) {
	result, err := AnalyzeSource("var x = Ghost()\nx")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %s, want none (F-64-1 tolerance)", codes(result))
	}
}

// §6.4: method bodies contribute their package-state references to the
// dependency closure - a method reading the variable closes a cycle
// through the receiver call.
func TestPackageVarMethodTransitiveCycle(t *testing.T) {
	src := "type Obj struct {\n}\nvar obj Obj = Obj{}\nvar x = obj.M()\nfunc (o Obj) M() int {\nreturn x\n}"
	result, err := AnalyzeSource(src)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCode(result, "ANUY3004") {
		t.Fatalf("diagnostics = %s, want ANUY3004", codes(result))
	}
}
