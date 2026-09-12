package integration

import (
	"github.com/san-smith/anuy/experimental/parser"
	"github.com/san-smith/anuy/experimental/semantic"
)

type Result struct{ Diagnostics []semantic.Diagnostic }

func AnalyzeSource(source string) (Result, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return Result{}, err
	}
	ops := make([]semantic.Operation, 0, len(program.Statements))
	bindings := map[string]semantic.BindingID{}
	var next semantic.BindingID
	for _, statement := range program.Statements {
		id, exists := bindings[statement.Name]
		switch statement.Kind {
		case parser.Var:
			next++
			id = next
			bindings[statement.Name] = id
			ops = append(ops, semantic.Declare(id))
		case parser.Assign:
			if exists {
				ops = append(ops, semantic.Assign(id))
			}
		case parser.Read:
			if exists {
				ops = append(ops, semantic.Read(id))
			}
		}
	}
	cfg := semantic.NewCFG(semantic.Block{ID: 1, Operations: ops})
	return Result{Diagnostics: (semantic.Analyzer{}).Analyze(cfg).Diagnostics}, nil
}
