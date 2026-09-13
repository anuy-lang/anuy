package integration

import (
	"github.com/san-smith/anuy/experimental/parser"
	"github.com/san-smith/anuy/experimental/semantic"
)

type Result struct{ Diagnostics []semantic.Diagnostic }

// AnalyzeSource parses the single-block experimental program, translates it to
// kernel operations and reports scope diagnostics. Multiple assignment emits
// right-hand side reads before left-hand side updates, so swap observes the
// parallel semantics of RFC-003 §61.
func AnalyzeSource(source string) (Result, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return Result{}, err
	}
	scope := semantic.NewScope()
	var ops []semantic.Operation
	var result Result
	for _, statement := range program.Statements {
		switch statement.Kind {
		case parser.Var:
			readValueIdents(scope, statement, &ops)
			declared := make([]string, 0, len(statement.Names))
			for _, name := range statement.Names {
				if serr := scope.Declare(name); serr != nil {
					result.Diagnostics = append(result.Diagnostics, diagnostic(serr.Category, statement.Span))
					continue
				}
				declared = append(declared, name)
				ops = append(ops, semantic.Declare(scope.Resolve(name)))
			}
			if statement.Values != nil {
				for _, name := range declared {
					ops = append(ops, semantic.Assign(scope.Resolve(name)))
				}
			}
		case parser.Assign:
			readValueIdents(scope, statement, &ops)
			for _, name := range statement.Names {
				if serr := scope.Assign(name); serr != nil {
					result.Diagnostics = append(result.Diagnostics, diagnostic(serr.Category, statement.Span))
					continue
				}
				ops = append(ops, semantic.Assign(scope.Resolve(name)))
			}
		case parser.Read:
			if id := scope.Resolve(statement.Names[0]); id != 0 {
				ops = append(ops, semantic.Read(id))
			}
		}
	}
	cfg := semantic.NewCFG(semantic.Block{ID: 1, Operations: ops})
	result.Diagnostics = append(result.Diagnostics, (semantic.Analyzer{}).Analyze(cfg).Diagnostics...)
	return result, nil
}

// readValueIdents emits reads for identifiers referenced by the right-hand
// side before any left-hand side operation, modeling "RHS values are
// evaluated before any LHS updates become observable" (RFC-003 §61).
// Declarations are not yet in scope for their own initializers (RFC-003 §36).
func readValueIdents(scope *semantic.Scope, statement parser.Statement, ops *[]semantic.Operation) {
	for _, idents := range statement.ValueIdents {
		for _, ident := range idents {
			if id := scope.Resolve(ident); id != 0 {
				*ops = append(*ops, semantic.Read(id))
			}
		}
	}
}

func diagnostic(category semantic.DiagnosticCategory, span parser.Span) semantic.Diagnostic {
	return semantic.NewDiagnostic(category, 0, semantic.SourceSpan{Start: span.Start, End: span.End})
}
