package integration

import (
	"sort"
	"strings"
	"unicode"

	"github.com/san-smith/anuy/experimental/parser"
	"github.com/san-smith/anuy/experimental/semantic"
)

type Result struct{ Diagnostics []semantic.Diagnostic }

// AnalyzeSource parses the experimental program and translates it into kernel
// CFGs. An if statement emits its condition reads in the branching block,
// each branch is emitted into its own child scope, and a dedicated join
// block receives edges from every continuing path, so initialization is
// proven on all of them (RFC-001 §141.4). A loop emits condition reads in
// its header, wires a body block over a child scope with a back edge to the
// header, and an exit join intersecting the zero-iteration and body paths
// (RFC-003 §70–75, §170.28–29). A closure literal is analyzed as
// its own CFG seeded with the facts at the creation point (RFC-001 §48,
// RFC-003 §82); closure operations never update caller facts
// (RFC-003 §84, §170.31).
func AnalyzeSource(source string) (Result, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return Result{}, err
	}
	b := newBuilder()
	b.emit(program.Statements, semantic.NewScope())
	result := Result{}
	result.Diagnostics = append(result.Diagnostics, b.diagnostics...)
	result.Diagnostics = append(result.Diagnostics, b.analyze().Diagnostics...)
	result.Diagnostics = append(result.Diagnostics, b.uncheckedErrors()...)
	return result, nil
}

// uncheckedErrors implements the R1 lint (CONTRACTS §3): a declared
// `error?` binding read nowhere yields exactly one Warning at its
// declaration span. Any read anywhere lifts it, assignments do not
// re-arm it (errcheck semantics), `_`-discards create no binding at all
// (GB-3), and nothing outside declared `error?` is checked. The order
// follows the declaration order (binding ids grow monotonically).
func (b *builder) uncheckedErrors() []semantic.Diagnostic {
	ids := make([]semantic.BindingID, 0, len(b.errSpans))
	for id := range b.errSpans {
		if !b.reads[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]semantic.Diagnostic, 0, len(ids))
	for _, id := range ids {
		span := b.errSpans[id]
		out = append(out, semantic.NewDiagnostic(semantic.UncheckedErrorDescriptor, id, semantic.SourceSpan{Start: span.Start, End: span.End}))
	}
	return out
}

type edge struct{ from, to semantic.BlockID }

// loopContext tracks the innermost open loop for `break`/`continue` wiring.
// exitFacts accumulates the intersection of facts on every edge entering the
// exit block: the zero-iteration path for condition/iteration loops plus each
// break point (RFC-003 §74). nil means no reachable path yet (an infinite
// loop without breaks).
type loopContext struct {
	header    semantic.BlockID
	exit      semantic.BlockID
	exitFacts map[semantic.BindingID]bool
	exitNN    map[semantic.BindingID]bool
}

type builder struct {
	blocks      []semantic.Block
	facts       []map[semantic.BindingID]bool // initialization facts at the end of each block
	nonNil      []map[semantic.BindingID]bool // non-nil narrowing facts, parallel to facts (story 05)
	edges       []edge
	cur         int // index into blocks
	nextBlockID semantic.BlockID
	loops       []loopContext
	diagnostics []semantic.Diagnostic
	// nilable marks bindings declared with a nullable `T?` type - the only
	// nilability evidence in the type-free experimental layer.
	nilable map[semantic.BindingID]bool
	// classes carries the static nullability class per binding (story 08,
	// G1 decision): declared named types classify by their `?`, untyped
	// declarations infer from the RHS (§4). The class is set once at the
	// declaration site and never changes with the flow; unknown is the
	// zero value and keeps platform semantics.
	classes map[semantic.BindingID]semantic.Nullability
	// flowNullable records bindings whose latest assignment classified
	// null (`x = nil`, Q3-A of the story 08 types proposal). No v1
	// consumer - the seed for future flow-sensitive lints; consumers bring
	// their own join plumbing.
	flowNullable map[semantic.BindingID]bool
	// closureMutates registers, per closure binding, the captured bindings
	// its body assigns (RFC-003 §85 invalidation).
	closureMutates map[semantic.BindingID][]semantic.BindingID
	// declared/assigned feed the mutator computation for closure bodies.
	declared map[semantic.BindingID]bool
	assigned map[semantic.BindingID]bool
	// errSpans records the declaration span of every declared `error?`
	// binding (R1 lint, CONTRACTS §3); reads tracks every binding read
	// anywhere in the flow (flow-insensitive "read somewhere" is enough).
	errSpans map[semantic.BindingID]parser.Span
	reads    map[semantic.BindingID]bool
	// pure records declared functions annotated `//anuy:pure` (story 07
	// effects proposal): a trusted contract - their calls apply no
	// mutator set.
	pure map[semantic.BindingID]bool
}

func newBuilder() *builder {
	return &builder{
		blocks:         []semantic.Block{{ID: 1}},
		facts:          []map[semantic.BindingID]bool{{}},
		nonNil:         []map[semantic.BindingID]bool{{}},
		nextBlockID:    1,
		nilable:        map[semantic.BindingID]bool{},
		classes:        map[semantic.BindingID]semantic.Nullability{},
		flowNullable:   map[semantic.BindingID]bool{},
		closureMutates: map[semantic.BindingID][]semantic.BindingID{},
		declared:       map[semantic.BindingID]bool{},
		assigned:       map[semantic.BindingID]bool{},
		errSpans:       map[semantic.BindingID]parser.Span{},
		reads:          map[semantic.BindingID]bool{},
		pure:           map[semantic.BindingID]bool{},
	}
}

func (b *builder) appendBlock() {
	b.nextBlockID++
	b.blocks = append(b.blocks, semantic.Block{ID: b.nextBlockID})
	b.facts = append(b.facts, map[semantic.BindingID]bool{})
	b.nonNil = append(b.nonNil, map[semantic.BindingID]bool{})
	b.cur = len(b.blocks) - 1
}

func (b *builder) add(op semantic.Operation) {
	switch op.Kind {
	case semantic.ReadOperation, semantic.DerefOperation:
		// Both operations read the binding's value: any of them lifts the
		// UncheckedError lint (CONTRACTS §3.3).
		b.reads[op.Binding] = true
	}
	b.blocks[b.cur].Operations = append(b.blocks[b.cur].Operations, op)
}

func (b *builder) declare(id semantic.BindingID) {
	b.add(semantic.Declare(id))
	b.facts[b.cur][id] = false
	b.nonNil[b.cur][id] = false
	b.declared[id] = true
}

func (b *builder) initialize(id semantic.BindingID) {
	b.add(semantic.Assign(id))
	b.facts[b.cur][id] = true
	// RFC-002 §25: assignment invalidates narrowing. Re-establishment (§26)
	// is the caller's move - the assignment path classifies the RHS and
	// emits the Assume after this invalidation (story 08).
	b.nonNil[b.cur][id] = false
	b.assigned[id] = true
}

// assume establishes the non-nil narrowing fact for the lowering of a
// proven `x != nil` predicate (RFC-002 §22).
func (b *builder) assume(id semantic.BindingID) {
	b.add(semantic.Assume(id))
	b.nonNil[b.cur][id] = true
}

// needsNonNilProof reports whether an ordinary member access on the binding
// requires the non-nil proof: a declared `T?` (story 05 nilable set) or an
// inferred-nullable class (story 08 inference, Q2-A). Unknown classes keep
// the platform semantics - dereference free, nothing established.
func (b *builder) needsNonNilProof(id semantic.BindingID) bool {
	return b.nilable[id] || b.classes[id] == semantic.NullabilityNullable
}

// classifyValue assigns the nullability class of an RHS value per the
// accepted classification (story 08 types proposal §3): non-nil literals,
// declared non-null bindings, live non-nil facts and non-null-classified
// bindings are non-null; `nil` and unproven nullable bindings are null;
// calls (no signatures in this slice), navigation, composite raw text and
// closures are unknown and never establish (CONTRACTS §2.1).
func (b *builder) classifyValue(value *parser.Value, scope *semantic.Scope) semantic.Nullability {
	if value == nil || value.Closure != nil || value.Navigation != nil {
		return semantic.NullabilityUnknown
	}
	if value.Text == "nil" {
		return semantic.NullabilityNullable
	}
	if len(value.Idents) == 1 && value.Text == value.Idents[0] {
		id := scope.Resolve(value.Idents[0])
		if id == 0 {
			return semantic.NullabilityUnknown
		}
		if b.nilable[id] {
			if b.nonNil[b.cur][id] {
				return semantic.NullabilityNonNull
			}
			return semantic.NullabilityNullable
		}
		return b.classes[id]
	}
	if len(value.Idents) == 0 && value.Text != "" {
		return semantic.NullabilityNonNull
	}
	return semantic.NullabilityUnknown
}

// establish applies §26 after the §25 invalidation of an assignment: a
// non-null-classified RHS re-establishes the narrowing (the Assume after
// the Assign), a null RHS records the flow-nullable fact (Q3-A, no
// consumers in v1), unknown changes nothing.
func (b *builder) establish(id semantic.BindingID, value *parser.Value, scope *semantic.Scope) {
	switch b.classifyValue(value, scope) {
	case semantic.NullabilityNonNull:
		b.assume(id)
	case semantic.NullabilityNullable:
		b.flowNullable[id] = true
	}
}

// establishAssignments pairs assignment targets with their RHS values by
// index and applies §26 (story 08). A name/value count mismatch leaves the
// extra targets unestablished - the tuple evaluation order is not modeled.
func (b *builder) establishAssignments(statement *parser.Statement, scope *semantic.Scope) {
	for i := range statement.Values {
		if i >= len(statement.Names) {
			return
		}
		name := statement.Names[i]
		if name == "_" {
			continue
		}
		if id := scope.Resolve(name); id != 0 {
			b.establish(id, &statement.Values[i], scope)
		}
	}
}

func (b *builder) isNonNil(id semantic.BindingID) bool {
	return b.nonNil[b.cur][id]
}

func (b *builder) isInitialized(id semantic.BindingID) bool {
	return b.facts[b.cur][id]
}

func (b *builder) connect(from, to semantic.BlockID) {
	b.edges = append(b.edges, edge{from: from, to: to})
}

// analyze runs the kernel analyzer over the built CFG.
func (b *builder) analyze() semantic.AnalysisResult {
	cfg := semantic.NewCFG(b.blocks...)
	for _, e := range b.edges {
		cfg.AddEdge(e.from, e.to)
	}
	return (semantic.Analyzer{}).Analyze(cfg)
}

func (b *builder) emit(statements []parser.Statement, scope *semantic.Scope) {
	for i := range statements {
		b.emitStatement(&statements[i], scope)
	}
}

func (b *builder) emitStatement(statement *parser.Statement, scope *semantic.Scope) {
	switch statement.Kind {
	case parser.Var:
		b.readIdents(statement, scope)
		declared := make([]string, 0, len(statement.Names))
		targets := make([]semantic.BindingID, 0, len(statement.Names))
		for i, name := range statement.Names {
			if name == "_" {
				// GB-3 variant A: the blank identifier receives a value but
				// creates no binding (RFC-003 §65).
				continue
			}
			// Inference (§4, story 08) reads the RHS class before the
			// declaration enters its own scope - matching readIdents, which
			// resolves initializer idents before the declaration (RFC-003 §36).
			var inferred semantic.Nullability
			if statement.TypeExpr == nil && i < len(statement.Values) {
				inferred = b.classifyValue(&statement.Values[i], scope)
			}
			if serr := scope.Declare(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			declared = append(declared, name)
			id := scope.Resolve(name)
			targets = append(targets, id)
			b.declare(id)
			if statement.TypeExpr != nil {
				if statement.TypeExpr.Nullable {
					b.nilable[id] = true
					if statement.TypeExpr.Name == "error" {
						b.errSpans[id] = statement.Span
					}
				}
				// Declared class (story 08, G1): a named type is non-null or
				// nullable by its `?`; composite spellings stay unknown -
				// their nullability binding is outside the slice.
				if statement.TypeExpr.Kind == parser.NamedType {
					if statement.TypeExpr.Nullable {
						b.classes[id] = semantic.NullabilityNullable
					} else {
						b.classes[id] = semantic.NullabilityNonNull
					}
				}
			} else {
				b.classes[id] = inferred
			}
		}
		if statement.Values != nil {
			for _, name := range declared {
				b.initialize(scope.Resolve(name))
			}
		}
		b.analyzeClosures(statement, scope, targets)
	case parser.Assign:
		b.readIdents(statement, scope)
		targets := make([]semantic.BindingID, 0, len(statement.Names))
		for _, name := range statement.Names {
			if name == "_" {
				// GB-3 variant A: a blank target receives the value without
				// creating or initializing any binding.
				continue
			}
			if serr := scope.Assign(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			targets = append(targets, scope.Resolve(name))
			b.initialize(scope.Resolve(name))
		}
		b.establishAssignments(statement, scope)
		b.analyzeClosures(statement, scope, targets)
	case parser.Read:
		b.readIdent(statement.Names[0], scope, statement.Span, nil)
	case parser.Call:
		b.emitCall(statement, scope)
	case parser.Function:
		b.emitFunction(statement, scope)
	case parser.Return:
		b.emitReturn()
	case parser.If:
		b.emitIf(statement, scope)
	case parser.Loop:
		b.emitLoop(statement, scope)
	case parser.Break:
		b.emitJump(true)
	case parser.Continue:
		b.emitJump(false)
	case parser.Block:
		// RFC-003 §25: the block creates a child scope; locals declared
		// inside do not survive it. Linear flow — no join block required.
		b.emit(statement.Body, scope.Child())
	}
}

// analyzeClosures analyzes closure literals of the statement at its creation
// point, after the left-hand side facts are recorded. targets are the
// binding ids the closure values are assigned to; the closure's mutated
// captures are registered there for RFC-003 §85 call invalidation.
func (b *builder) analyzeClosures(statement *parser.Statement, scope *semantic.Scope, targets []semantic.BindingID) {
	index := 0
	for _, value := range statement.Values {
		if value.Closure == nil {
			continue
		}
		mutators := b.analyzeClosure(value.Closure, scope)
		if index < len(targets) {
			// Решение 1 (2026-09-16): union across all assignments of the
			// binding — a later closure must not erase an earlier mutator.
			b.closureMutates[targets[index]] = unionBindings(b.closureMutates[targets[index]], mutators)
		}
		index++
	}
	b.propagateMutators(statement, scope, targets)
}

func (b *builder) emitIf(statement *parser.Statement, scope *semantic.Scope) {
	// Condition reads evaluate in the branching block, before any branch.
	b.readConditionIdents(statement, scope)
	before := copyFacts(b.facts[b.cur])
	beforeNN := copyFacts(b.nonNil[b.cur])
	start := b.blocks[b.cur].ID
	b.appendBlock()
	thenID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(before)
	b.nonNil[b.cur] = copyFacts(beforeNN)
	if narrowID := b.condNarrowTarget(statement, scope); narrowID != 0 {
		b.assume(narrowID)
	}
	b.emit(statement.Body, scope.Child())
	thenFacts := copyFacts(b.facts[b.cur])
	thenNN := copyFacts(b.nonNil[b.cur])
	thenExit := b.blocks[b.cur].ID

	hasElse := statement.Else != nil
	var elseID, elseExit semantic.BlockID
	var elseFacts, elseNN map[semantic.BindingID]bool
	if hasElse {
		b.appendBlock()
		elseID = b.blocks[b.cur].ID
		b.facts[b.cur] = copyFacts(before)
		b.nonNil[b.cur] = copyFacts(beforeNN)
		b.emit(statement.Else, scope.Child())
		elseFacts = copyFacts(b.facts[b.cur])
		elseNN = copyFacts(b.nonNil[b.cur])
		elseExit = b.blocks[b.cur].ID
	}

	joinFacts := intersectFacts(thenFacts, before)
	joinNN := intersectFacts(thenNN, beforeNN)
	if hasElse {
		joinFacts = intersectFacts(thenFacts, elseFacts)
		joinNN = intersectFacts(thenNN, elseNN)
	}

	b.appendBlock()
	b.facts[b.cur] = joinFacts
	b.nonNil[b.cur] = joinNN
	joinID := b.blocks[b.cur].ID
	b.connect(start, thenID)
	b.connect(thenExit, joinID)
	if hasElse {
		b.connect(start, elseID)
		b.connect(elseExit, joinID)
	} else {
		b.connect(start, joinID)
	}
}

// emitLoop wires a loop CFG (spec 1-3-1-1, RFC-003 §70–78): condition and
// collection reads evaluate in the header on every iteration, the body block
// starts from header facts so reads are proven per iteration (loop-carried),
// and the exit keeps a binding initialized only on paths that actually reach
// it — the zero-iteration path plus every break point (§170.28–29). The back
// edge body→header lets the analyzer's fixpoint downgrade facts that only
// the body establishes. An infinite loop (`for { }`) has no condition exit:
// its exit is reachable only through break edges and stays fully
// unreachable until one exists (no proofs required, RFC-001 §39).
func (b *builder) emitLoop(statement *parser.Statement, scope *semantic.Scope) {
	entryID := b.blocks[b.cur].ID
	b.appendBlock()
	headerID := b.blocks[b.cur].ID
	b.readConditionIdents(statement, scope)
	if len(statement.Values) > 0 {
		// The iteration collection evaluates in the header on every
		// iteration (RFC-003 §76). Collection resolution has no binding
		// model yet, so an unresolved collection stays invisible here.
		for _, ident := range statement.Values[0].Idents {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
		}
	}
	headerFacts := copyFacts(b.facts[b.cur])
	headerNN := copyFacts(b.nonNil[b.cur])

	b.appendBlock()
	exitIdx := b.cur
	exitID := b.blocks[exitIdx].ID
	// Only condition/iteration loops have a zero-iteration exit path; an
	// infinite loop's exit is reachable exclusively through breaks. The flag
	// is captured before the body because breaks replace the nil exitFacts.
	hasConditionExit := statement.Cond != "" || len(statement.Values) > 0
	var exitFacts map[semantic.BindingID]bool
	var exitNN map[semantic.BindingID]bool
	if hasConditionExit {
		exitFacts = copyFacts(headerFacts)
		exitNN = copyFacts(headerNN)
	}

	b.appendBlock()
	bodyID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(headerFacts)
	b.nonNil[b.cur] = copyFacts(headerNN)
	// A bare `x != nil` loop condition re-establishes the narrowing for the
	// body on every iteration; the exit path stays unproven.
	if narrowID := b.condNarrowTarget(statement, scope); narrowID != 0 {
		b.assume(narrowID)
	}
	bodyScope := scope.Child()
	if len(statement.Names) > 0 {
		// RFC-003 §76, §27: the binding lives in the loop's child scope and
		// is initialized for each logical iteration; it shadows outer names
		// (§78, §170.20) and is invisible after the loop. The static single
		// binding records §77/§170.32 per-iteration identity only as an
		// observation (integration tests), not as a mechanism.
		if serr := bodyScope.Declare(statement.Names[0]); serr != nil {
			b.report(serr.Category, statement.Span)
		} else {
			id := bodyScope.Resolve(statement.Names[0])
			b.declare(id)
			b.initialize(id)
		}
	}
	b.loops = append(b.loops, loopContext{header: headerID, exit: exitID, exitFacts: exitFacts, exitNN: exitNN})
	b.emit(statement.Body, bodyScope)
	loopCtx := &b.loops[len(b.loops)-1]
	b.connect(b.blocks[b.cur].ID, headerID)
	b.loops = b.loops[:len(b.loops)-1]

	b.facts[exitIdx] = loopCtx.exitFacts
	b.nonNil[exitIdx] = loopCtx.exitNN
	b.cur = exitIdx // post-loop statements emit in the exit/join block

	b.connect(entryID, headerID)
	b.connect(headerID, bodyID)
	if hasConditionExit {
		b.connect(headerID, exitID)
	}
}

// emitJump wires `break` (edge to the loop exit) and `continue` (edge to the
// loop header), both targeting the nearest enclosing loop. The jump edge
// carries exactly the facts at the jump point: statements after the jump
// start an unreachable block, and every break point intersects its facts
// into the exit (RFC-003 §74 — only reachable exits contribute to
// post-loop state).
func (b *builder) emitJump(isBreak bool) {
	if len(b.loops) == 0 {
		return // unreachable: the parser rejects jumps outside loops
	}
	ctx := &b.loops[len(b.loops)-1]
	curFacts := copyFacts(b.facts[b.cur])
	curNN := copyFacts(b.nonNil[b.cur])
	if isBreak {
		b.connect(b.blocks[b.cur].ID, ctx.exit)
		if ctx.exitFacts == nil {
			ctx.exitFacts = curFacts
		} else {
			ctx.exitFacts = intersectFacts(ctx.exitFacts, curFacts)
		}
		if ctx.exitNN == nil {
			ctx.exitNN = curNN
		} else {
			ctx.exitNN = intersectFacts(ctx.exitNN, curNN)
		}
	} else {
		b.connect(b.blocks[b.cur].ID, ctx.header)
	}
	b.appendBlock()
	b.facts[b.cur] = curFacts
	b.nonNil[b.cur] = curNN
}

// readIdents emits reads for identifiers referenced by right-hand side
// expressions before any left-hand side operation, modeling "RHS values are
// evaluated before any LHS updates become observable" (RFC-003 §61).
// Declarations are not yet in scope for their own initializers (RFC-003 §36).
// Closure values carry no top-level idents; their bodies are analyzed as
// separate CFGs (RFC-003 §80–84). Unresolved RHS idents — including call
// callees, for which the grammar has no declaration form yet — stay
// invisible here: recorded conformance sources pin them as accepted.
func (b *builder) readIdents(statement *parser.Statement, scope *semantic.Scope) {
	for _, value := range statement.Values {
		if value.Closure != nil {
			continue
		}
		for _, ident := range value.Idents {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
		}
		// An ordinary member access on a declared `T?` receiver requires
		// the non-nil proof (RFC-002 §22; safe tails are exempt via
		// SafeNavigate). Unresolved receivers stay in the F-G3 tolerance
		// zone.
		if value.Navigation != nil && len(value.Navigation.Segments) > 0 && !value.Navigation.Segments[0].Safe {
			if id := scope.Resolve(value.Navigation.Receiver); id != 0 && b.needsNonNilProof(id) {
				b.add(semantic.Deref(id))
			}
		}
	}
}

// readConditionIdents emits the condition reads of an if/loop header. A
// condition that is exactly one bare identifier is a binding read: unresolved,
// it reports one UnknownRead (D-01). Compound conditions keep calls and
// navigation opaque — the grammar has no declaration form for callees yet, so
// only their resolved idents become reads.
func (b *builder) readConditionIdents(statement *parser.Statement, scope *semantic.Scope) {
	reported := map[string]bool{}
	for _, ident := range statement.CondIdents {
		if statement.Cond != ident {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
			continue
		}
		b.readIdent(ident, scope, statement.Span, reported)
	}
}

// readIdent emits the read of a name that resolves through the scope chain
// and reports exactly one UnknownRead per unresolved name at the read span
// (D-01, the RFC-003 §14 analogue on the read side). ReadBeforeInitialization
// stays reserved for resolved bindings lacking definite initialization, and
// the assignment path is untouched. reported dedupes repeated names within
// one statement; nil works for single-name sites.
func (b *builder) readIdent(name string, scope *semantic.Scope, span parser.Span, reported map[string]bool) {
	if serr := scope.Read(name); serr != nil {
		if !reported[name] {
			if reported != nil {
				reported[name] = true
			}
			b.report(serr.Category, span)
		}
		return
	}
	b.add(semantic.Read(scope.Resolve(name)))
}

// analyzeClosure models a closure body as its own CFG whose entry facts are
// the facts at the closure creation point:
//
//   - parameters are initialized at entry (RFC-003 §86);
//   - if the body reads a captured binding, that binding must be definitely
//     initialized at creation: an uninitialized capture is seeded
//     uninitialized, so the body analysis reports the read (RFC-001 §48–49,
//     RFC-003 §82);
//   - a non-nil capture is seeded with its narrowing (CONTRACTS §1.5, the
//     narrowing analogue of the initialization seeding);
//   - a write-only capture of an uninitialized binding is allowed and its
//     assignment initializes it inside the closure flow (RFC-003 §83);
//   - closure operations never update caller facts (RFC-003 §84, §170.31).
//
// It returns the bindings the body assigns without declaring - the mutated
// captures, for §85 call invalidation.
func (b *builder) analyzeClosure(cl *parser.Closure, scope *semantic.Scope) []semantic.BindingID {
	bodyScope := scope.Child()
	var entry []semantic.Operation
	paramIDs := map[semantic.BindingID]bool{}
	for _, p := range cl.Params {
		if serr := bodyScope.Declare(p.Name); serr != nil {
			b.report(serr.Category, cl.Span)
			continue
		}
		id := bodyScope.Resolve(p.Name)
		paramIDs[id] = true
		// §27 stable bindings: parameter types carry the declared class
		// (story 08) - a `T?` parameter gates ordinary member access.
		b.classes[id] = nullabilityOfParam(p)
		entry = append(entry, semantic.Declare(id), semantic.Assign(id))
	}
	seen := map[semantic.BindingID]bool{}
	for _, ident := range closureIdents(cl.Body) {
		id := bodyScope.Resolve(ident)
		if id == 0 || paramIDs[id] || seen[id] {
			continue
		}
		seen[id] = true
		entry = append(entry, semantic.Declare(id))
		if b.isInitialized(id) {
			entry = append(entry, semantic.Assign(id))
		}
		if b.isNonNil(id) {
			entry = append(entry, semantic.Assume(id))
		}
	}
	cb := &builder{
		blocks:       []semantic.Block{{ID: 1, Operations: entry}},
		facts:        []map[semantic.BindingID]bool{{}},
		nonNil:       []map[semantic.BindingID]bool{{}},
		nextBlockID:  1,
		nilable:      b.nilable,
		classes:      b.classes,
		flowNullable: b.flowNullable,
		declared:     map[semantic.BindingID]bool{},
		assigned:     map[semantic.BindingID]bool{},
		errSpans:     map[semantic.BindingID]parser.Span{},
		reads:        map[semantic.BindingID]bool{},
		pure:         b.pure,
	}
	cb.emit(cl.Body, bodyScope)
	// Closure bodies participate in the same lint and read accounting:
	// a read anywhere lifts the warning, a closure-local `error?`
	// declaration is linted with the rest (CONTRACTS §3).
	for id := range cb.reads {
		b.reads[id] = true
	}
	for id, span := range cb.errSpans {
		b.errSpans[id] = span
	}
	var mutators []semantic.BindingID
	for id := range cb.assigned {
		if !cb.declared[id] {
			mutators = append(mutators, id)
		}
	}
	b.diagnostics = append(b.diagnostics, cb.diagnostics...)
	b.diagnostics = append(b.diagnostics, cb.analyze().Diagnostics...)
	return mutators
}

// propagateMutators implements Решение 2 (2026-09-16): assigning a closure
// value held by another binding propagates its mutator set to the target
// (`var c2 = clear; c2()` invalidates the narrowing). Only the unambiguous
// single-target bare-identifier form propagates; call values, navigation
// chains and compound values are not aliasing sources.
func (b *builder) propagateMutators(statement *parser.Statement, scope *semantic.Scope, targets []semantic.BindingID) {
	if len(statement.Values) != 1 || len(targets) != 1 {
		return
	}
	value := statement.Values[0]
	if value.Closure != nil || value.Navigation != nil || len(value.Idents) != 1 || value.Text != value.Idents[0] {
		return
	}
	source := scope.Resolve(value.Idents[0])
	if source == 0 || source == targets[0] {
		return
	}
	if mutators := b.closureMutates[source]; len(mutators) > 0 {
		b.closureMutates[targets[0]] = unionBindings(b.closureMutates[targets[0]], mutators)
	}
}

func unionBindings(existing, added []semantic.BindingID) []semantic.BindingID {
	seen := make(map[semantic.BindingID]bool, len(existing)+len(added))
	out := make([]semantic.BindingID, 0, len(existing)+len(added))
	for _, id := range existing {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range added {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// emitCall lowers a call statement: arguments evaluate first (RFC-003 §61),
// the bare callee is a binding read, and the call of a declared function or
// closure applies its mutator set (RFC-003 §85). A navigation call requires
// the non-nil proof for its ordinary prefix and invalidates the receiver's
// narrowing after the call (story 07, решение 3b — the Rust &mut self
// analogue); its method name is never a binding read (F-G3 boundary). An
// annotated `//anuy:pure` callee applies no mutator set (trusted contract).
func (b *builder) emitCall(statement *parser.Statement, scope *semantic.Scope) {
	call := statement.Call
	// A bare-identifier argument is a read (D-01): unresolved, it reports
	// exactly one UnknownRead per name. Compound and navigation arguments
	// keep the readIdents tolerance - unresolved idents there stay invisible
	// (recorded conformance sources pin them as accepted).
	reported := map[string]bool{}
	for _, value := range statement.Values {
		if value.Navigation != nil || value.Closure != nil || len(value.Idents) != 1 || value.Text != value.Idents[0] {
			continue
		}
		if serr := scope.Read(value.Idents[0]); serr != nil && !reported[value.Idents[0]] {
			reported[value.Idents[0]] = true
			b.report(serr.Category, statement.Span)
		}
	}
	b.readIdents(statement, scope)
	b.analyzeClosures(statement, scope, nil)
	id := scope.Resolve(call.Receiver)
	if id == 0 {
		if len(call.Segments) == 0 {
			b.report(semantic.UnknownRead, statement.Span)
		}
		return
	}
	b.add(semantic.Read(id))
	if len(call.Segments) > 0 {
		if !call.Segments[0].Safe && b.needsNonNilProof(id) {
			b.add(semantic.Deref(id))
		}
		// Решение 3b: the proof is required at the call point (the Deref
		// above) and dropped after it - chained calls need re-proof.
		b.add(semantic.Call(id))
		return
	}
	if mutators := b.closureMutates[id]; len(mutators) > 0 && !b.pure[id] {
		b.add(semantic.Call(mutators...))
	}
}

// emitFunction lowers a story 07 function declaration: the declared name
// binds a closure value, so the body is analyzed at the creation point like
// a closure literal - self-recursion resolves through the scope, and the
// body's mutated captures register in the mutator registry (CONTRACTS
// story 07 §1). `//anuy:pure` marks the trusted no-writes contract.
func (b *builder) emitFunction(statement *parser.Statement, scope *semantic.Scope) {
	if serr := scope.Declare(statement.Names[0]); serr != nil {
		b.report(serr.Category, statement.Span)
		return
	}
	id := scope.Resolve(statement.Names[0])
	b.declare(id)
	b.initialize(id)
	mutators := b.analyzeClosure(statement.Closure, scope)
	b.closureMutates[id] = unionBindings(b.closureMutates[id], mutators)
	if statement.Pure {
		b.pure[id] = true
	}
}

// emitReturn lowers the bare `return` statement (story 07): the flow of the
// enclosing function terminates here. Following statements start a fresh
// block with no incoming edges - the analyzer skips unreachable blocks, so
// their reads report nothing.
func (b *builder) emitReturn() {
	cur := b.cur
	b.appendBlock()
	b.facts[b.cur] = copyFacts(b.facts[cur])
	b.nonNil[b.cur] = copyFacts(b.nonNil[cur])
}

// condNarrowTarget resolves the binding narrowed by a bare `x != nil`
// condition - the only nil-comparison form in the experimental slice;
// conditions otherwise stay raw text until the general expression grammar
// exists. Zero means the condition carries no narrowing.
func (b *builder) condNarrowTarget(statement *parser.Statement, scope *semantic.Scope) semantic.BindingID {
	parts := strings.SplitN(statement.Cond, "!=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return 0
	}
	name := strings.TrimSpace(parts[0])
	for _, ident := range statement.CondIdents {
		if ident == name {
			return scope.Resolve(ident)
		}
	}
	return 0
}

// closureIdents collects identifiers referenced anywhere in the statements,
// including nested closures, in source order. Assignment and declaration
// targets are not reads and are not collected.
func closureIdents(statements []parser.Statement) []string {
	var out []string
	for i := range statements {
		s := &statements[i]
		out = append(out, s.CondIdents...)
		for _, value := range s.Values {
			if value.Closure == nil {
				out = append(out, value.Idents...)
				continue
			}
			out = append(out, closureIdents(value.Closure.Body)...)
		}
		if s.Kind == parser.Read {
			out = append(out, s.Names...)
		}
		// A call statement's bare callee and navigation receiver are binding
		// reads (CONTRACTS §2): they participate in capture seeding.
		if s.Kind == parser.Call && s.Call != nil {
			out = append(out, s.Call.Receiver)
		}
		out = append(out, closureIdents(s.Body)...)
		out = append(out, closureIdents(s.Else)...)
	}
	return out
}

func copyFacts(facts map[semantic.BindingID]bool) map[semantic.BindingID]bool {
	out := make(map[semantic.BindingID]bool, len(facts))
	for id, initialized := range facts {
		out[id] = initialized
	}
	return out
}

// intersectFacts keeps a binding initialized only if both incoming paths
// initialize it, matching the kernel join semantics.
func intersectFacts(a, b map[semantic.BindingID]bool) map[semantic.BindingID]bool {
	out := make(map[semantic.BindingID]bool)
	for id, initialized := range a {
		if initialized && b[id] {
			out[id] = true
		}
	}
	return out
}

func (b *builder) report(category semantic.DiagnosticCategory, span parser.Span) {
	desc, ok := semantic.DescriptorFor(category)
	if !ok {
		panic("integration: unregistered diagnostic category " + category)
	}
	b.diagnostics = append(b.diagnostics, semantic.NewDiagnostic(desc, 0, semantic.SourceSpan{Start: span.Start, End: span.End}))
}

// nullabilityOfParam classifies a declared parameter type (story 08): a
// named type is non-null or nullable by its `?`; composite spellings stay
// unknown - their nullability binding is outside the slice.
func nullabilityOfParam(p parser.Param) semantic.Nullability {
	if p.TypeExpr != nil {
		if p.TypeExpr.Kind == parser.NamedType {
			if p.TypeExpr.Nullable {
				return semantic.NullabilityNullable
			}
			return semantic.NullabilityNonNull
		}
		return semantic.NullabilityUnknown
	}
	return nullabilityOfTypeName(p.Type)
}

// nullabilityOfTypeName classifies a raw type-name spelling: a simple name
// is non-null, the same with a trailing `?` is nullable, everything else
// (slice/map/pointer composites) stays unknown.
func nullabilityOfTypeName(t string) semantic.Nullability {
	base := strings.TrimSuffix(t, "?")
	if base == t {
		if isSimpleTypeName(t) {
			return semantic.NullabilityNonNull
		}
		return semantic.NullabilityUnknown
	}
	if isSimpleTypeName(base) {
		return semantic.NullabilityNullable
	}
	return semantic.NullabilityUnknown
}

func isSimpleTypeName(t string) bool {
	if t == "" {
		return false
	}
	for i, r := range t {
		if !unicode.IsLetter(r) && r != '_' && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
