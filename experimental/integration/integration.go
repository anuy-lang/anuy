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
		switch statement.Kind {
		case parser.Var:
			for _, name := range statement.Names {
				next++
				id := next
				bindings[name] = id
				ops = append(ops, semantic.Declare(id))
				if statement.Values != nil {
					ops = append(ops, semantic.Assign(id))
				}
			}
		case parser.Assign:
			for _, name := range statement.Names {
				if id, exists := bindings[name]; exists {
					ops = append(ops, semantic.Assign(id))
				}
			}
		case parser.Read:
			if id, exists := bindings[statement.Names[0]]; exists {
				ops = append(ops, semantic.Read(id))
			}
		}
	}
	cfg := semantic.NewCFG(semantic.Block{ID: 1, Operations: ops})
	return Result{Diagnostics: (semantic.Analyzer{}).Analyze(cfg).Diagnostics}, nil
}
