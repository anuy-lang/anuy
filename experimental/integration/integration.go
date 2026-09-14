package integration

import (
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
	return result, nil
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
}

type builder struct {
	blocks      []semantic.Block
	facts       []map[semantic.BindingID]bool // initialization facts at the end of each block
	edges       []edge
	cur         int // index into blocks
	nextBlockID semantic.BlockID
	loops       []loopContext
	diagnostics []semantic.Diagnostic
}

func newBuilder() *builder {
	return &builder{
		blocks:      []semantic.Block{{ID: 1}},
		facts:       []map[semantic.BindingID]bool{{}},
		nextBlockID: 1,
	}
}

func (b *builder) appendBlock() {
	b.nextBlockID++
	b.blocks = append(b.blocks, semantic.Block{ID: b.nextBlockID})
	b.facts = append(b.facts, map[semantic.BindingID]bool{})
	b.cur = len(b.blocks) - 1
}

func (b *builder) add(op semantic.Operation) {
	b.blocks[b.cur].Operations = append(b.blocks[b.cur].Operations, op)
}

func (b *builder) declare(id semantic.BindingID) {
	b.add(semantic.Declare(id))
	b.facts[b.cur][id] = false
}

func (b *builder) initialize(id semantic.BindingID) {
	b.add(semantic.Assign(id))
	b.facts[b.cur][id] = true
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
		for _, name := range statement.Names {
			if serr := scope.Declare(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			declared = append(declared, name)
			b.declare(scope.Resolve(name))
		}
		if statement.Values != nil {
			for _, name := range declared {
				b.initialize(scope.Resolve(name))
			}
		}
		b.analyzeClosures(statement, scope)
	case parser.Assign:
		b.readIdents(statement, scope)
		for _, name := range statement.Names {
			if serr := scope.Assign(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			b.initialize(scope.Resolve(name))
		}
		b.analyzeClosures(statement, scope)
	case parser.Read:
		if id := scope.Resolve(statement.Names[0]); id != 0 {
			b.add(semantic.Read(id))
		}
	case parser.If:
		b.emitIf(statement, scope)
	case parser.Loop:
		b.emitLoop(statement, scope)
	case parser.Break:
		b.emitJump(true)
	case parser.Continue:
		b.emitJump(false)
	}
}

// analyzeClosures analyzes closure literals of the statement at its creation
// point, after the left-hand side facts are recorded.
func (b *builder) analyzeClosures(statement *parser.Statement, scope *semantic.Scope) {
	for _, value := range statement.Values {
		if value.Closure != nil {
			b.analyzeClosure(value.Closure, scope)
		}
	}
}

func (b *builder) emitIf(statement *parser.Statement, scope *semantic.Scope) {
	// Condition reads evaluate in the branching block, before any branch.
	for _, ident := range statement.CondIdents {
		if id := scope.Resolve(ident); id != 0 {
			b.add(semantic.Read(id))
		}
	}
	before := copyFacts(b.facts[b.cur])
	start := b.blocks[b.cur].ID
	b.appendBlock()
	thenID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(before)
	b.emit(statement.Body, scope.Child())
	thenFacts := copyFacts(b.facts[b.cur])
	thenExit := b.blocks[b.cur].ID

	hasElse := statement.Else != nil
	var elseID, elseExit semantic.BlockID
	var elseFacts map[semantic.BindingID]bool
	if hasElse {
		b.appendBlock()
		elseID = b.blocks[b.cur].ID
		b.facts[b.cur] = copyFacts(before)
		b.emit(statement.Else, scope.Child())
		elseFacts = copyFacts(b.facts[b.cur])
		elseExit = b.blocks[b.cur].ID
	}

	joinFacts := intersectFacts(thenFacts, before)
	if hasElse {
		joinFacts = intersectFacts(thenFacts, elseFacts)
	}

	b.appendBlock()
	b.facts[b.cur] = joinFacts
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
	for _, ident := range statement.CondIdents {
		if id := scope.Resolve(ident); id != 0 {
			b.add(semantic.Read(id))
		}
	}
	if len(statement.Values) > 0 {
		// The iteration collection evaluates in the header on every
		// iteration (RFC-003 §76).
		for _, ident := range statement.Values[0].Idents {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
		}
	}
	headerFacts := copyFacts(b.facts[b.cur])

	b.appendBlock()
	exitIdx := b.cur
	exitID := b.blocks[exitIdx].ID
	// Only condition/iteration loops have a zero-iteration exit path; an
	// infinite loop's exit is reachable exclusively through breaks. The flag
	// is captured before the body because breaks replace the nil exitFacts.
	hasConditionExit := statement.Cond != "" || len(statement.Values) > 0
	var exitFacts map[semantic.BindingID]bool
	if hasConditionExit {
		exitFacts = copyFacts(headerFacts)
	}

	b.appendBlock()
	bodyID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(headerFacts)
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
	b.loops = append(b.loops, loopContext{header: headerID, exit: exitID, exitFacts: exitFacts})
	b.emit(statement.Body, bodyScope)
	loopCtx := &b.loops[len(b.loops)-1]
	b.connect(b.blocks[b.cur].ID, headerID)
	b.loops = b.loops[:len(b.loops)-1]

	b.facts[exitIdx] = loopCtx.exitFacts
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
	if isBreak {
		b.connect(b.blocks[b.cur].ID, ctx.exit)
		if ctx.exitFacts == nil {
			ctx.exitFacts = curFacts
		} else {
			ctx.exitFacts = intersectFacts(ctx.exitFacts, curFacts)
		}
	} else {
		b.connect(b.blocks[b.cur].ID, ctx.header)
	}
	b.appendBlock()
	b.facts[b.cur] = curFacts
}

// readIdents emits reads for identifiers referenced by right-hand side
// expressions before any left-hand side operation, modeling "RHS values are
// evaluated before any LHS updates become observable" (RFC-003 §61).
// Declarations are not yet in scope for their own initializers (RFC-003 §36).
// Closure values carry no top-level idents; their bodies are analyzed as
// separate CFGs (RFC-003 §80–84).
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
	}
}

// analyzeClosure models a closure body as its own CFG whose entry facts are
// the facts at the closure creation point:
//
//   - parameters are initialized at entry (RFC-003 §86);
//   - if the body reads a captured binding, that binding must be definitely
//     initialized at creation: an uninitialized capture is seeded
//     uninitialized, so the body analysis reports the read (RFC-001 §48–49,
//     RFC-003 §82);
//   - a write-only capture of an uninitialized binding is allowed and its
//     assignment initializes it inside the closure flow (RFC-003 §83);
//   - closure operations never update caller facts (RFC-003 §84, §170.31).
func (b *builder) analyzeClosure(cl *parser.Closure, scope *semantic.Scope) {
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
	}
	cb := &builder{
		blocks:      []semantic.Block{{ID: 1, Operations: entry}},
		facts:       []map[semantic.BindingID]bool{{}},
		nextBlockID: 1,
	}
	cb.emit(cl.Body, bodyScope)
	b.diagnostics = append(b.diagnostics, cb.diagnostics...)
	b.diagnostics = append(b.diagnostics, cb.analyze().Diagnostics...)
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
	b.diagnostics = append(b.diagnostics, semantic.NewDiagnostic(category, 0, semantic.SourceSpan{Start: span.Start, End: span.End}))
}
