package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/experimental/parser"
	"github.com/anuy-lang/anuy/experimental/semantic"
)

func assertSingleDiagnostic(t *testing.T, result Result, want string) {
	t.Helper()
	if len(result.Diagnostics) != 1 || string(result.Diagnostics[0].Category) != want {
		t.Fatalf("result = %#v, want one %s diagnostic", result, want)
	}
}

func TestAnalyzeSourceReportsReadBeforeInitialization(t *testing.T) {
	result, err := AnalyzeSource("var x int\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceReportsUnknownAssignment(t *testing.T) {
	result, err := AnalyzeSource("var x int\ny = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownAssignment")
}

func TestAnalyzeSourceReportsSameScopeRedeclaration(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar x int\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SameScopeRedeclaration")
}

func TestAnalyzeSourceReportsUnknownRead(t *testing.T) {
	// D-01/P-25: a bare identifier that resolves through no scope reports
	// exactly one UnknownRead at the read span.
	result, err := AnalyzeSource("x\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
	if result.Diagnostics[0].Span.Start != 0 || result.Diagnostics[0].Span.End != 1 {
		t.Fatalf("span = %#v, want the read span 0..1", result.Diagnostics[0].Span)
	}
}

func TestAnalyzeSourceReportsUnknownReadInIfCondition(t *testing.T) {
	result, err := AnalyzeSource("if ready {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}

func TestAnalyzeSourceReportsUnknownReadInLoopCondition(t *testing.T) {
	result, err := AnalyzeSource("for ready {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}

func TestAnalyzeSourceDoesNotTreatNavigationMembersAsBindingReads(t *testing.T) {
	result, err := AnalyzeSource("var user int = 1\nvar address int\nvar city = user.address?.city\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceAcceptsParallelSwap(t *testing.T) {
	result, err := AnalyzeSource("var x int = 1\nvar y int = 2\nx, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceReportsUninitializedSwapOperand(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar y int = 2\nx, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceAcceptsMultipleDeclarationWithInitializer(t *testing.T) {
	result, err := AnalyzeSource("var x, y = f(), g()\nx\ny\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceAcceptsIfElseInitializingBothBranches(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nif ready {\nx = 1\n} else {\nx = 2\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceReportsBranchMissingAssignment(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nif ready {\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceReportsUninitializedConditionRead(t *testing.T) {
	result, err := AnalyzeSource("var c int\nif c {\nc = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceScopesBranchLocals(t *testing.T) {
	result, err := AnalyzeSource("var ready int = 1\nif ready {\nvar t int\n}\nt = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownAssignment")
}

func TestAnalyzeSourceAcceptsBranchShadowing(t *testing.T) {
	result, err := AnalyzeSource("var x int = 1\nvar ready int = 1\nif ready {\nvar x int\nx = 2\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceAcceptsClosureReadingInitializedCapture(t *testing.T) {
	result, err := AnalyzeSource("var handler int = 1\nvar callback = func() {\nhandler\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceReportsCaptureReadBeforeInitialization(t *testing.T) {
	result, err := AnalyzeSource("var handler int\nvar callback = func() {\nhandler\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceClosureWriteDoesNotProveCallerInitialization(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar init = func() {\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceAcceptsWriteOnlyCaptureOfUninitializedBinding(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar init = func() {\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceAcceptsCaptureReadAfterDominatingAssignment(t *testing.T) {
	// D-02/P-26 (F-17): the capture needs definite initialization at closure
	// creation only on a path that reads it before a dominating assignment in
	// the closure body, so `x = 1; x` is valid even though the outer x was
	// uninitialized at creation (RFC-001 §48, RFC-003 §83).
	result, err := AnalyzeSource("var x int\nvar init = func() {\nx = 1\nx\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceClosureBodyLocalsFollowInitializationRules(t *testing.T) {
	result, err := AnalyzeSource("var f = func() {\nvar count int\ncount\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceClosureParameterShadowsOuter(t *testing.T) {
	result, err := AnalyzeSource("var x int\nvar f = func(x int) {\nx = 1\nx\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceCapturePreservesBindingIdentity(t *testing.T) {
	result, err := AnalyzeSource("var count int = 0\nvar increment = func() {\ncount = count + 1\n}\ncount\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceRejectsZeroIterationLoopAssignment(t *testing.T) {
	// RFC-003 §70, §170.28: a loop that may execute zero times does not
	// establish post-loop initialization by body assignment alone.
	result, err := AnalyzeSource("var ready int = 1\nvar x int\nfor ready {\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceAcceptsInitializationBeforeLoop(t *testing.T) {
	// RFC-003 §71: initialized before the loop, the binding stays proven.
	result, err := AnalyzeSource("var x int = 0\nvar ready int = 1\nfor ready {\nx = update(x)\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceRejectsLoopCarriedReadBeforeAssignment(t *testing.T) {
	// RFC-003 §75: the first iteration may read before initialization.
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nfor ready {\nx\nx = 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceRejectsUninitializedLoopConditionRead(t *testing.T) {
	// RFC-001 §141.3: condition reads evaluate in the header and require
	// definite initialization.
	result, err := AnalyzeSource("var c int\nfor c {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceAcceptsLoopBodyReadAfterAssignment(t *testing.T) {
	// A read after an assignment on the same path is safe on every iteration.
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nfor ready {\nx = 1\nx\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceIterationBindingIsInitializedPerIteration(t *testing.T) {
	// RFC-003 §76: the binding is created initialized for each logical
	// iteration, so reading it in the body needs no proof.
	result, err := AnalyzeSource("for user in users {\nuser\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceIterationBindingShadowsOuter(t *testing.T) {
	// RFC-003 §78, §170.20/27: the loop binding shadows the outer name and
	// leaves the outer binding untouched.
	result, err := AnalyzeSource("var user = defaultUser()\nfor user in users {\nuser\n}\nuser\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceIterationBindingNotVisibleAfterLoop(t *testing.T) {
	// RFC-003 §27: the binding belongs to the loop construct; after the loop
	// the name resolves to nothing, so an assignment is unknown.
	result, err := AnalyzeSource("for user in users {\n}\nuser = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownAssignment")
}

func TestAnalyzeSourceIterationBindingAssignmentInsideBody(t *testing.T) {
	// RFC-003 §76: the binding is an ordinary initialized mutable binding
	// inside the body; reassignment resolves and needs no diagnostic.
	result, err := AnalyzeSource("for user in users {\nuser = next(user)\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceInfiniteLoopAllBreakPathsInitialize(t *testing.T) {
	// RFC-003 §72: every reachable exit assigns the binding, so the post-loop
	// read is proven (two break paths, both initializing).
	result, err := AnalyzeSource("var x int\nfor {\nif cancel() {\nx = fallback()\nbreak\n}\nx = readValue()\nif valid(x) {\nbreak\n}\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceInfiniteLoopRejectsBreakPathWithoutAssignment(t *testing.T) {
	// RFC-003 §73: one reachable exit leaves the binding uninitialized.
	result, err := AnalyzeSource("var x int\nfor {\nif cancel() {\nbreak\n}\nx = readValue()\nif valid(x) {\nbreak\n}\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceContinueDoesNotContributeToLoopExit(t *testing.T) {
	// RFC-003 §74: continue only loops back; the condition exit carries
	// header facts, where x is still uninitialized (§70).
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nfor ready {\nif bad() {\ncontinue\n}\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceInfiniteLoopContinueSkipsToNextIteration(t *testing.T) {
	// RFC-003 §74: continue targets the header; the only reachable exit is
	// the break after the assignment.
	result, err := AnalyzeSource("var x int\nfor {\nif bad() {\ncontinue\n}\nx = readValue()\nbreak\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceIterationBindingClosureCaptureObservation(t *testing.T) {
	// RFC-003 §77/§170.32 observation: per-iteration logical identity is a
	// SHOULD the static single-CFG does not model — the closure capture
	// refers to the single static binding of the loop (§79). Choosing a
	// mechanism is out of scope; this test only records that the capture is
	// seeded initialized (§76) and reports no diagnostic.
	result, err := AnalyzeSource("for user in users {\nvar f = func() {\nuser\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceNestedLoopBreakBindsToNearestLoop(t *testing.T) {
	// The inner break exits the inner loop only, so x = 2 stays reachable on
	// the outer body path and the outer break exit proves x for the post-loop
	// read (nearest-loop binding, no labeled transfer). With the break
	// wrongly bound to the outer loop, x = 2 would be unreachable and the
	// read would report ReadBeforeInitialization.
	result, err := AnalyzeSource("var x int\nfor {\nfor {\nbreak\n}\nx = 2\nbreak\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceConditionLoopBreakPathInitializedAtEntry(t *testing.T) {
	// §71+§74: with x initialized at entry, both exits — the zero-iteration
	// condition exit and the break exit — carry x initialized.
	result, err := AnalyzeSource("var x int = 0\nvar ready int = 1\nfor ready {\nif bad() {\nbreak\n}\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceConditionLoopIntersectsBreakAndConditionExits(t *testing.T) {
	// §70+§74: the exit initializes x only if BOTH the condition exit and the
	// break path initialize it; neither does, so the post-loop read reports.
	result, err := AnalyzeSource("var x int\nvar ready int = 1\nfor ready {\nif bad() {\nbreak\n}\nx = 1\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceBlockLocalsDoNotLeak(t *testing.T) {
	// RFC-003 §25: block-local declarations do not survive the block; the
	// name resolves to nothing afterwards, so an assignment is unknown.
	result, err := AnalyzeSource("var x int = 1\n{\nvar y int = 2\n}\ny = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownAssignment")
}

func TestAnalyzeSourceBlockShadowingIsolated(t *testing.T) {
	// RFC-003 §30–31, §170.27: the block-local x is a new binding; its
	// initialization does not touch the outer x, which stays uninitialized.
	result, err := AnalyzeSource("var x int\n{\nvar x int = 2\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "ReadBeforeInitialization")
}

func TestAnalyzeSourceBlankDiscardCreatesNoBinding(t *testing.T) {
	// GB-3 variant A: `_` receives a value position but creates no binding;
	// the named sibling is initialized normally.
	result, err := AnalyzeSource("var value, _ = f()\nvalue\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceBlankTargetAssignsNothing(t *testing.T) {
	// `_ = 1` and blank targets in a list emit no assignment operation and
	// never report UnknownAssignment.
	result, err := AnalyzeSource("var x int\n_ = 1\n_, x = 2, 3\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceElseIfChainJoinsAllBranches(t *testing.T) {
	// D-03: else if is sugar for a nested if, so the kernel join proves every
	// branch of the chain initializes x.
	result, err := AnalyzeSource("var x int\nvar a int = 1\nvar b int = 1\nif a {\nx = 1\n} else if b {\nx = 2\n} else {\nx = 3\n}\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result)
	}
}

func TestAnalyzeSourceReportsClosureMutationInvalidatingNarrowing(t *testing.T) {
	// RFC-003 §85 end-to-end: clear() may assign the same binding the
	// compiler proved non-nil, so the subsequent user.save() is rejected.
	result, err := AnalyzeSource("var user User? = findUser()\nvar clear = func() {\nuser = nil\n}\nif user != nil {\nclear()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAcceptsMemberAccessAfterNilNarrowing(t *testing.T) {
	// RFC-002 §22: inside the proven branch the ordinary member access is safe.
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignmentInvalidatesNarrowing(t *testing.T) {
	// RFC-002 §25 at source level: an assignment inside the proven branch
	// invalidates the narrowing (conservative - the experimental layer has
	// no RHS types, so RFC-002 §26 establishment is out of slice).
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nuser = findUser()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceReadonlyCaptureDoesNotInvalidateNarrowing(t *testing.T) {
	result, err := AnalyzeSource("var user User? = findUser()\nvar clear = func() {\nuser\n}\nif user != nil {\nclear()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceSeedsCapturedNonNilAtClosureCreation(t *testing.T) {
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nvar f = func() {\nuser.save()\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceBodyAssignmentInvalidatesCapturedNarrowing(t *testing.T) {
	result, err := AnalyzeSource("var user User? = findUser()\nif user != nil {\nvar f = func() {\nuser = nil\nuser.save()\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceDerefWithoutNarrowingReports(t *testing.T) {
	result, err := AnalyzeSource("var user User? = findUser()\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceNonNullableReceiverNeedsNoNarrowing(t *testing.T) {
	// The non-nil requirement attaches to declared `T?` receivers only; a
	// non-nullable declared type can never be nil.
	result, err := AnalyzeSource("var user User = getUser()\nuser.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceReportsUnknownCalleeStatement(t *testing.T) {
	// A bare call callee is a binding read (proposal variant A, D-01).
	result, err := AnalyzeSource("findUser()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}

func TestAnalyzeSourceUnionMutatorsAcrossReassignment(t *testing.T) {
	// Решение 1 (2026-09-16): the mutator registry unions across all
	// assignments of a binding — a later read-only rebinding must not erase
	// the earlier mutator, whatever the order.
	result, err := AnalyzeSource("var user User? = findUser()\nvar clear = func() {\nuser = nil\n}\nclear = func() {\nuser\n}\nif user != nil {\nclear()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourcePropagatesMutatorsThroughAliasing(t *testing.T) {
	// Решение 2: `var c2 = clear` propagates the mutator set to c2, so the
	// aliased call still invalidates the narrowing.
	result, err := AnalyzeSource("var user User? = findUser()\nvar clear = func() {\nuser = nil\n}\nvar c2 = clear\nif user != nil {\nc2()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceReadonlyAliasingDoesNotInvalidate(t *testing.T) {
	result, err := AnalyzeSource("var user User? = findUser()\nvar clear = func() {\nuser\n}\nvar c2 = clear\nif user != nil {\nc2()\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceReportsUncheckedError(t *testing.T) {
	// R1 / CONTRACTS §3: a declared `error?` binding read nowhere yields
	// exactly one Warning at the declaration span.
	result, err := AnalyzeSource("var err error? = f()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 {
		t.Fatalf("result = %#v, want one UncheckedError", result.Diagnostics)
	}
	d := result.Diagnostics[0]
	if d.Category != "UncheckedError" || d.Code != "ANUY5001" || d.Severity != semantic.SeverityWarning {
		t.Fatalf("diagnostic = (%s, %s, %s), want (UncheckedError, ANUY5001, Warning)", d.Category, d.Code, d.Severity)
	}
	if d.Span.Start != 0 || d.Span.End != len("var err error? = f()") {
		t.Fatalf("span = %v, want the declaration statement span", d.Span)
	}
}

func TestAnalyzeSourceUncheckedErrorReadLifts(t *testing.T) {
	result, err := AnalyzeSource("var err error? = f()\nvar logged = err\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics once the binding is read", result.Diagnostics)
	}
}

func TestAnalyzeSourceUncheckedErrorConditionReadLifts(t *testing.T) {
	result, err := AnalyzeSource("var err error? = f()\nvar v int = 1\nif err != nil {\nv\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics: the nil condition is a read", result.Diagnostics)
	}
}

func TestAnalyzeSourceUncheckedErrorAssignmentDoesNotReArm(t *testing.T) {
	// errcheck semantics: assignments without a read neither lift the
	// warning nor add a second one.
	result, err := AnalyzeSource("var err error? = f()\nerr = g()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY5001" {
		t.Fatalf("result = %#v, want exactly one UncheckedError", result.Diagnostics)
	}
}

func TestAnalyzeSourceBlankErrorDiscardExempt(t *testing.T) {
	// GB-3: `_` creates no binding, so there is nothing to lint.
	result, err := AnalyzeSource("var _ error? = f()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics for the blank discard", result.Diagnostics)
	}
}

func TestAnalyzeSourceNonErrorTypesNotLinted(t *testing.T) {
	// CONTRACTS §3.4: nothing outside declared `error?` is checked - plain
	// and nilable non-error bindings stay out of the lint.
	result, err := AnalyzeSource("var v int = 1\nvar u User? = get()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics for non-error bindings", result.Diagnostics)
	}
}

func TestAnalyzeSourceTwoUncheckedErrorsReportEachOnce(t *testing.T) {
	// One Warning per declaration, in declaration order.
	result, err := AnalyzeSource("var a error? = f()\nvar b error? = g()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 2 {
		t.Fatalf("result = %#v, want one UncheckedError per binding", result.Diagnostics)
	}
	for i, want := range []string{"a", "b"} {
		_ = want
		if d := result.Diagnostics[i]; d.Code != "ANUY5001" || d.Binding != semantic.BindingID(i+1) {
			t.Fatalf("diagnostic %d = (%s, binding %d), want (ANUY5001, binding %d)", i, d.Code, d.Binding, i+1)
		}
	}
}

func TestAnalyzeSourceAssignLiteralEstablishesNonNull(t *testing.T) {
	// §26 (story 08): a non-nil literal RHS re-establishes the narrowing
	// after the §25 invalidation - the deref is proven again.
	result, err := AnalyzeSource("var u User? = find()\nif u != nil {\nu = 42\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignDeclaredNonNullEstablishes(t *testing.T) {
	// §26: a declared non-null binding as RHS establishes the fact.
	result, err := AnalyzeSource("var u User? = find()\nvar nn User = make()\nif u != nil {\nu = nn\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignProvenBindingEstablishes(t *testing.T) {
	// §26: a nilable binding with a live non-nil fact (proven on this path)
	// classifies non-null and re-establishes the target.
	result, err := AnalyzeSource("var u User? = find()\nvar p User? = find()\nif p != nil {\nif u != nil {\nu = p\nu.save()\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignInferredNonNullEstablishes(t *testing.T) {
	// Inference (§4): an untyped declaration classifies by its RHS; a
	// non-null-classified binding re-establishes the target.
	result, err := AnalyzeSource("var n = 42\nvar u User? = find()\nif u != nil {\nu = n\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignNullThenNonNullEstablishes(t *testing.T) {
	// §25 then §26, in order: the null assignment clears, the following
	// non-null assignment re-establishes.
	result, err := AnalyzeSource("var u User? = find()\nvar nn User = make()\nif u != nil {\nu = nil\nu = nn\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceAssignEstablishDoesNotSurviveJoin(t *testing.T) {
	// Established on the then-path only (no outer proof): the join
	// downgrades to unknown and the deref after the join stays unproven.
	result, err := AnalyzeSource("var u User? = find()\nvar nn User = make()\nvar c bool = true\nif c {\nu = nn\n}\nu.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAssignEstablishInvalidatedByFollowingUnknown(t *testing.T) {
	// §25 first: an unknown call after the establishment clears it again.
	result, err := AnalyzeSource("var u User? = find()\nvar nn User = make()\nif u != nil {\nu = nn\nu = find()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAssignNilDoesNotEstablish(t *testing.T) {
	// Negative control: `nil` never establishes.
	result, err := AnalyzeSource("var u User? = find()\nif u != nil {\nu = nil\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAssignUnknownCallDoesNotEstablish(t *testing.T) {
	// Negative control (RFC-002 §26 reproducer without result types): an
	// unknown call stays unknown - nothing is established.
	result, err := AnalyzeSource("var u User? = findUser()\nif u != nil {\nu = createUser()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAssignNavigationDoesNotEstablish(t *testing.T) {
	// Negative control: navigation RHS is unknown (no field model, §28).
	result, err := AnalyzeSource("var u User? = find()\nvar a Account? = find()\nif a != nil {\nif u != nil {\nu = a.owner\nu.save()\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceDeclarationEstablishes(t *testing.T) {
	// ADR-0001 (RFC-003 §13.8: first initialization uses ordinary `=`):
	// a non-null initializer establishes per §6.3.5 - the literal
	// classifies non-null, so the deref is proven without narrowing.
	result, err := AnalyzeSource("var u User? = 42\nu.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceDeclarationUnknownCallDoesNotEstablish(t *testing.T) {
	// Declaration-path negative control: an unknown call initializer stays
	// unknown - nothing is established.
	result, err := AnalyzeSource("var u User? = find()\nu.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceDeclarationNilDoesNotEstablish(t *testing.T) {
	// Declaration-path negative control: `nil` never establishes.
	result, err := AnalyzeSource("var u User? = nil\nu.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceAssignAfterDeclarationInvalidates(t *testing.T) {
	// §25: the assignment after the established declaration invalidates;
	// an unknown RHS does not re-establish.
	result, err := AnalyzeSource("var u User? = 42\nu = find()\nu.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceEstablishedCaptureSeedsClosure(t *testing.T) {
	// The establishment at the creation point seeds the closure entry: the
	// captured binding keeps its non-nil narrowing (CONTRACTS §1.5).
	result, err := AnalyzeSource("var u User? = 42\nvar f = func() {\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceInferredNullableRequiresProof(t *testing.T) {
	// Inference (Q2-A): an untyped binding initialized from a nullable value
	// classifies nullable - the deref requires the proof.
	result, err := AnalyzeSource("var u User? = find()\nvar x = u\nx.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceInferredNilRequiresProof(t *testing.T) {
	// Inference: `var x = nil` is nullable - the deref requires the proof.
	result, err := AnalyzeSource("var x = nil\nx.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceInferredNullableNarrowingLiftsProof(t *testing.T) {
	// The inferred-nullable binding narrows like a declared one: inside the
	// proven branch the deref is fine.
	result, err := AnalyzeSource("var u User? = find()\nvar x = u\nif x != nil {\nx.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceInferredUnknownCallIsPlatformFree(t *testing.T) {
	// Unknown keeps platform semantics (Kotlin T!): the deref stays free
	// and nothing is established.
	result, err := AnalyzeSource("var x = find()\nx.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceInferredLiteralIsFree(t *testing.T) {
	result, err := AnalyzeSource("var x = 42\nx.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceNullableClosureParamRequiresProof(t *testing.T) {
	// Declared `T?` parameters carry the nullable class (§27 stable
	// bindings): the ordinary deref on the parameter requires the proof.
	result, err := AnalyzeSource("var h = func(p User?) {\np.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAssignNullRecordsFlowNullableFact(t *testing.T) {
	// Q3-A (story 08 types proposal): a null-classified assignment records
	// the flow-nullable fact. No v1 consumer - the write is the seed for
	// future flow-sensitive lints (CONTRACTS §2.4: tests reference facts).
	program, err := parser.Parse("var u User? = find()\nu = nil\n")
	if err != nil {
		t.Fatal(err)
	}
	scope := semantic.NewScope()
	b := newBuilder()
	b.emit(program.Statements, scope)
	id := scope.Resolve("u")
	if id == 0 {
		t.Fatal("binding u not resolved")
	}
	if !b.flowNullable[id] {
		t.Fatalf("flowNullable[%d] = false, want true after `u = nil`", id)
	}
	// A non-null-classified assignment does not record the fact.
	program, err = parser.Parse("var u User? = find()\nu = 42\n")
	if err != nil {
		t.Fatal(err)
	}
	scope = semantic.NewScope()
	b = newBuilder()
	b.emit(program.Statements, scope)
	if b.flowNullable[scope.Resolve("u")] {
		t.Fatalf("flowNullable after `u = 42` = true, want false")
	}
}

func TestAnalyzeSourcePureMethodKeepsReceiverNarrowing(t *testing.T) {
	// F-C2 / Q2-A: a `//anuy:pure` method opts out of the receiver
	// invalidation (3b) - the proof survives the call.
	result, err := AnalyzeSource("//anuy:pure\nfunc T.m() {\n}\nvar u User? = find()\nif u != nil {\nu.m()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceMethodMutatorsInvalidateCaptures(t *testing.T) {
	// 3a for methods (Q1-A composition): the call invalidates the narrowing
	// of the bindings the method body assigns.
	result, err := AnalyzeSource("var u User? = find()\nvar c User? = find()\nfunc T.m() {\nc = nil\n}\nif u != nil {\nif c != nil {\nu.m()\nc.save()\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceDuplicateMethodReports(t *testing.T) {
	// Q1-A: the flat method namespace - a duplicate method name rejects.
	result, err := AnalyzeSource("func T.m() {\n}\nfunc U.m() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SameScopeRedeclaration")
}

func TestAnalyzeSourceMethodNameCollisionWithFunctionReports(t *testing.T) {
	// Q1-A: one flat namespace - a method name colliding with a declared
	// function rejects, in both orders.
	result, err := AnalyzeSource("func f() {\n}\nfunc T.f() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SameScopeRedeclaration")
	result, err = AnalyzeSource("func T.g() {\n}\nfunc g() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SameScopeRedeclaration")
}

func TestAnalyzeSourceBareMethodNameIsUnknownRead(t *testing.T) {
	// Q1-A: methods bind no scope name - the bare call stays an unresolved
	// callee (D-01), not a function call.
	result, err := AnalyzeSource("func T.m() {\n}\nm()\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnknownRead")
}

func TestAnalyzeSourceSafeCallNeedsNoProof(t *testing.T) {
	// §33: the safe call requires no receiver proof; the call itself still
	// invalidates the narrowing (the method may write).
	result, err := AnalyzeSource("var u User? = find()\nfunc T.m() {\n}\nu?.m()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceSafeCallInvalidatesReceiverNarrowing(t *testing.T) {
	// §33 desugaring: the call happens on the taken path, so the narrowing
	// is dropped afterwards (3b applies to the safe form too).
	result, err := AnalyzeSource("var u User? = find()\nfunc T.m() {\n}\nif u != nil {\nu?.m()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceKnownNonNullCallResultEstablishes(t *testing.T) {
	// §26 full reproducer (Q4-A): `createUser() -> User` classifies the RHS
	// non-null and re-establishes the narrowing.
	result, err := AnalyzeSource("func createUser() User {\nreturn make()\n}\nvar u User? = find()\nif u != nil {\nu = createUser()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceKnownNullableCallResultDoesNotEstablish(t *testing.T) {
	// A `T?` result classifies null - the assignment stays unproven.
	result, err := AnalyzeSource("func findUser() User? {\nreturn make()\n}\nvar u User? = find()\nif u != nil {\nu = findUser()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceVoidCallResultDoesNotEstablish(t *testing.T) {
	// A void call has no result value - unknown, nothing establishes.
	result, err := AnalyzeSource("func run() {\n}\nvar u User? = find()\nif u != nil {\nu = run()\nu.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "UnsafeMemberAccess")
}

func TestAnalyzeSourceMethodResultClassifiesRHS(t *testing.T) {
	// A declared method's result type feeds the call-shape classification:
	// `c = c.clone()` with `func T.clone() User` re-establishes.
	result, err := AnalyzeSource("var c User? = find()\nfunc T.clone() User {\nreturn make()\n}\nif c != nil {\nc = c.clone()\nc.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceReturnValueReadLiftsUncheckedErrorLint(t *testing.T) {
	// `return expr` reads participate in the flow: the read lifts the R1
	// lint on the returned binding.
	result, err := AnalyzeSource("var err error? = make()\nfunc find() error? {\nreturn err\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceMissingReturnReported(t *testing.T) {
	// D-6 (RFC-001 §8.2.6, ADR-0004): a declared non-null result must be
	// initialized on all exit paths - falling off the end of the body is
	// one Error at the declaration span.
	result, err := AnalyzeSource("func find() User {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "MissingReturn")
}

func TestAnalyzeSourceAllPathsReturnNoDiagnostic(t *testing.T) {
	result, err := AnalyzeSource("func find(u User) User {\nif u != nil {\nreturn u\n}\nreturn u\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceVoidFallOffAllowed(t *testing.T) {
	// ADR-0004: void/no-result functions may fall off the end.
	result, err := AnalyzeSource("func f() {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceNullableResultFallOffAllowed(t *testing.T) {
	// A nullable result binding falls off to semantic nil - a valid value;
	// only non-null results are D-6.
	result, err := AnalyzeSource("func find() User? {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceInfiniteLoopWithoutExitTerminates(t *testing.T) {
	// No path reaches the body end, so the result cannot fall off
	// (Go reference: `for {}` satisfies the return requirement).
	result, err := AnalyzeSource("func f() int {\nfor {\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceBreakMakesFallOffReachable(t *testing.T) {
	// The break edge reaches the loop exit and the body end from there.
	result, err := AnalyzeSource("func f() int {\nfor {\nbreak\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "MissingReturn")
}

func TestAnalyzeSourceReturnInsideLoopCoversPaths(t *testing.T) {
	result, err := AnalyzeSource("func f() int {\nfor {\nreturn 1\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("result = %#v, want no diagnostics", result.Diagnostics)
	}
}

func TestAnalyzeSourceIfWithoutElseFallsOff(t *testing.T) {
	// The join after a condition if is reachable from the header - the
	// zero-branch path falls off the body end.
	result, err := AnalyzeSource("func f() int {\nvar x int = 1\nif x != nil {\nreturn 1\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "MissingReturn")
}

func TestAnalyzeSourceMethodMissingReturnReported(t *testing.T) {
	// Methods (story 08) enforce D-6 like functions.
	result, err := AnalyzeSource("func User.age() int {\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "MissingReturn")
}
