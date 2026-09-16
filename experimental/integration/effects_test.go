package integration

import (
	"testing"

	"github.com/san-smith/anuy/experimental/semantic"
)

func TestAnalyzeSourceDeclaredFunctionCallResolves(t *testing.T) {
	// A declared function is a binding with a closure value: the callee is a
	// read, the mutator set is empty - no diagnostics.
	result, err := AnalyzeSource("func f() {\n}\nf()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceForwardReferenceReportsUnknownRead(t *testing.T) {
	// Declare-before-use (owner decision 2026-09-17): the body of g sees no
	// binding f yet - D-01 reports the read.
	result, err := AnalyzeSource("func g() {\nf()\n}\nfunc f() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}

func TestAnalyzeSourceFunctionRedeclarationReports(t *testing.T) {
	result, err := AnalyzeSource("func f() {\n}\nfunc f() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SameScopeRedeclaration")
}

func TestAnalyzeSourceDeclaredMutatorsInvalidateNarrowing(t *testing.T) {
	// Решение 3a: calling a declared function applies its mutator set - the
	// story 05 e2e scenario through a declaration instead of a closure.
	result, err := AnalyzeSource("var user User? = findUser()\nfunc clear() {\nuser = nil\n}\nif user != nil {\nclear()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
	if got := result.Diagnostics[0]; got.Code != "ANUY4001" || got.Severity != semantic.SeverityError {
		t.Fatalf("diagnostic = (%s, %s), want (ANUY4001, Error)", got.Code, got.Severity)
	}
}

func TestAnalyzeSourcePureDeclaredFunctionDoesNotInvalidate(t *testing.T) {
	// //anuy:pure is a trusted author contract (NullAway model): the call
	// applies no mutator set, so the narrowing survives.
	result, err := AnalyzeSource("var user User? = findUser()\n//anuy:pure\nfunc noop() {\nuser = nil\n}\nif user != nil {\nnoop()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics for a pure call", result.Diagnostics)
	}
}

func TestAnalyzeSourceMethodCallInvalidatesReceiverNarrowing(t *testing.T) {
	// Решение 3b (следствие принято 2026-09-17): each ordinary method call
	// drops the receiver proof - the chained call needs re-proof.
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nuser.save()\nuser.send()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceSingleMethodCallStaysValid(t *testing.T) {
	// RFC-002 §22 stays valid: the proof is required at the call point and
	// only dropped after it.
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceReturnStopsFlow(t *testing.T) {
	// Bare return terminates the flow: the read after it is unreachable and
	// reports nothing.
	result, err := AnalyzeSource("var u int\nfunc f() {\nreturn\nu\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics after return", result.Diagnostics)
	}
}

func TestAnalyzeSourceCallArgumentsAreReads(t *testing.T) {
	// Arguments are reads of their identifiers; a resolved callee is not
	// reported, an unresolved argument is (D-01).
	result, err := AnalyzeSource("func f() {\n}\nf(missing)\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}
