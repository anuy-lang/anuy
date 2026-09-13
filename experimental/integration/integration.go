package integration

import (
	"github.com/san-smith/anuy/experimental/parser"
	"github.com/san-smith/anuy/experimental/semantic"
)

type Result struct{ Diagnostics []semantic.Diagnostic }

// AnalyzeSource parses the experimental program and translates it into a
// kernel CFG: an if statement emits its condition reads in the branching
// block, each branch is emitted into its own child scope, and a dedicated
// join block receives edges from every continuing path, so initialization is
// proven on all of them (RFC-001 §141.4).
func AnalyzeSource(source string) (Result, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return Result{}, err
	}
	b := newBuilder()
	b.emit(program.Statements, semantic.NewScope())
	cfg := semantic.NewCFG(b.blocks...)
	for _, e := range b.edges {
		cfg.AddEdge(e.from, e.to)
	}
	result := Result{}
	result.Diagnostics = append(result.Diagnostics, b.diagnostics...)
	result.Diagnostics = append(result.Diagnostics, (semantic.Analyzer{}).Analyze(cfg).Diagnostics...)
	return result, nil
}

type edge struct{ from, to semantic.BlockID }

type builder struct {
	blocks      []semantic.Block
	edges       []edge
	cur         int // index into blocks
	nextBlockID semantic.BlockID
	diagnostics []semantic.Diagnostic
}

func newBuilder() *builder {
	return &builder{blocks: []semantic.Block{{ID: 1}}, nextBlockID: 1}
}

func (b *builder) appendBlock() {
	b.nextBlockID++
	b.blocks = append(b.blocks, semantic.Block{ID: b.nextBlockID})
	b.cur = len(b.blocks) - 1
}

func (b *builder) add(op semantic.Operation) {
	b.blocks[b.cur].Operations = append(b.blocks[b.cur].Operations, op)
}

func (b *builder) connect(from, to semantic.BlockID) {
	b.edges = append(b.edges, edge{from: from, to: to})
}

func (b *builder) emit(statements []parser.Statement, scope *semantic.Scope) {
	for i := range statements {
		b.emitStatement(&statements[i], scope)
	}
}

func (b *builder) emitStatement(statement *parser.Statement, scope *semantic.Scope) {
	switch statement.Kind {
	case parser.Var:
		b.readIdents(statement.ValueIdents, scope)
		declared := make([]string, 0, len(statement.Names))
		for _, name := range statement.Names {
			if serr := scope.Declare(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			declared = append(declared, name)
			b.add(semantic.Declare(scope.Resolve(name)))
		}
		if statement.Values != nil {
			for _, name := range declared {
				b.add(semantic.Assign(scope.Resolve(name)))
			}
		}
	case parser.Assign:
		b.readIdents(statement.ValueIdents, scope)
		for _, name := range statement.Names {
			if serr := scope.Assign(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			b.add(semantic.Assign(scope.Resolve(name)))
		}
	case parser.Read:
		if id := scope.Resolve(statement.Names[0]); id != 0 {
			b.add(semantic.Read(id))
		}
	case parser.If:
		b.emitIf(statement, scope)
	}
}

func (b *builder) emitIf(statement *parser.Statement, scope *semantic.Scope) {
	// Condition reads evaluate in the branching block, before any branch.
	for _, ident := range statement.CondIdents {
		if id := scope.Resolve(ident); id != 0 {
			b.add(semantic.Read(id))
		}
	}
	start := b.blocks[b.cur].ID
	b.appendBlock()
	thenID := b.blocks[b.cur].ID
	b.emit(statement.Body, scope.Child())
	thenExit := b.blocks[b.cur].ID

	hasElse := statement.Else != nil
	var elseID, elseExit semantic.BlockID
	if hasElse {
		b.appendBlock()
		elseID = b.blocks[b.cur].ID
		b.emit(statement.Else, scope.Child())
		elseExit = b.blocks[b.cur].ID
	}

	b.appendBlock()
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

// readIdents emits reads for identifiers referenced by right-hand side
// expressions before any left-hand side operation, modeling "RHS values are
// evaluated before any LHS updates become observable" (RFC-003 §61).
// Declarations are not yet in scope for their own initializers (RFC-003 §36).
func (b *builder) readIdents(idents [][]string, scope *semantic.Scope) {
	for _, group := range idents {
		for _, ident := range group {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
		}
	}
}

func (b *builder) report(category semantic.DiagnosticCategory, span parser.Span) {
	b.diagnostics = append(b.diagnostics, semantic.NewDiagnostic(category, 0, semantic.SourceSpan{Start: span.Start, End: span.End}))
}
