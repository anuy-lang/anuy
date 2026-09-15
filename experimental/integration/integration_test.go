package integration

import "testing"

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
