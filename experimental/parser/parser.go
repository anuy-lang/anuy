// Package parser implements a deliberately restricted experimental grammar.
package parser

import (
	"fmt"
	"strings"
)

type Span struct{ Start, End int }
type Kind uint8

const (
	Var Kind = iota
	Assign
	If
	Read
)

type Statement struct {
	Kind Kind
	Name string
	Span Span
}
type Program struct{ Statements []Statement }

// Parse accepts only `var <name>`, `<name> = <literal>`, and `if <name>` lines.
func Parse(source string) (Program, error) {
	var p Program
	offset := 0
	for _, line := range strings.SplitAfter(source, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			offset += len(line)
			continue
		}
		fields := strings.Fields(text)
		st := Statement{Span: Span{Start: offset, End: offset + len(text)}}
		switch {
		case len(fields) == 2 && fields[0] == "var":
			st.Kind = Var
			st.Name = fields[1]
		case len(fields) == 3 && fields[1] == "=":
			st.Kind = Assign
			st.Name = fields[0]
		case len(fields) == 2 && fields[0] == "if":
			st.Kind = If
			st.Name = fields[1]
		case len(fields) == 1:
			st.Kind = Read
			st.Name = fields[0]
		default:
			return Program{}, fmt.Errorf("experimental parser: unsupported syntax at %d", offset)
		}
		p.Statements = append(p.Statements, st)
		offset += len(line)
	}
	return p, nil
}
