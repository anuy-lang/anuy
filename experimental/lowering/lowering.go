package lowering

import (
	"fmt"
	"strings"

	"github.com/san-smith/anuy/experimental/parser"
)

// Lower emits deliberately minimal experimental Go for accepted narrow syntax.
func Lower(source string) (string, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return "", err
	}
	var body strings.Builder
	last, err := lowerStatements(&body, program.Statements)
	if err != nil {
		return "", err
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	return "package fixture\n\nfunc Run() {\n" + body.String() + "}\n", nil
}

func lowerStatements(body *strings.Builder, statements []parser.Statement) (string, error) {
	var last string
	for _, statement := range statements {
		names := strings.Join(statement.Names, ", ")
		values := make([]string, 0, len(statement.Values))
		for _, value := range statement.Values {
			text, err := lowerValue(value)
			if err != nil {
				return "", err
			}
			values = append(values, text)
		}
		switch statement.Kind {
		case parser.Var:
			if statement.Type != "" && strings.Contains(statement.Type, "?") {
				return "", fmt.Errorf("experimental lowering: unsupported nullable type %q", statement.Type)
			}
			switch {
			case statement.Type != "" && len(values) == 0:
				fmt.Fprintf(body, "\tvar %s %s\n", names, statement.Type)
			case statement.Type != "":
				fmt.Fprintf(body, "\tvar %s %s = %s\n", names, statement.Type, strings.Join(values, ", "))
			default:
				fmt.Fprintf(body, "\t%s := %s\n", names, strings.Join(values, ", "))
			}
			last = statement.Names[len(statement.Names)-1]
		case parser.Assign:
			fmt.Fprintf(body, "\t%s = %s\n", names, strings.Join(values, ", "))
			last = statement.Names[len(statement.Names)-1]
		case parser.Read:
			fmt.Fprintf(body, "\t_ = %s\n", statement.Names[0])
			last = statement.Names[0]
		case parser.If:
			fmt.Fprintf(body, "\tif %s {\n", statement.Cond)
			if _, err := lowerStatements(body, statement.Body); err != nil {
				return "", err
			}
			if statement.Else != nil {
				body.WriteString("\t} else {\n")
				if _, err := lowerStatements(body, statement.Else); err != nil {
					return "", err
				}
			}
			body.WriteString("\t}\n")
		case parser.Loop:
			switch {
			case len(statement.Names) > 0:
				// Iteration form lowers to Go range with a discarded index
				// (spec 1-3-1-1 lowering plan); the binding is fresh on every
				// iteration, matching §76/§77.
				operand, err := rangeOperand(statement.Values[0])
				if err != nil {
					return "", err
				}
				fmt.Fprintf(body, "\tfor _, %s := range %s {\n", statement.Names[0], operand)
				if _, err := lowerStatements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			case statement.Cond != "":
				fmt.Fprintf(body, "\tfor %s {\n", statement.Cond)
				if _, err := lowerStatements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			default:
				body.WriteString("\tfor {\n")
				if _, err := lowerStatements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			}
		case parser.Call:
			// Story 05: the effectful statement form - the call value is
			// discarded by statement semantics, so no blank discard is
			// appended and nothing feeds the trailing `_ =` line.
			expr := statement.Call.Receiver
			for _, segment := range statement.Call.Segments {
				expr += "." + segment.Name
			}
			fmt.Fprintf(body, "\t%s()\n", expr)
			last = ""
		case parser.Break:
			body.WriteString("\tbreak\n")
		case parser.Continue:
			body.WriteString("\tcontinue\n")
		case parser.Block:
			body.WriteString("\t{\n")
			if _, err := lowerStatements(body, statement.Body); err != nil {
				return "", err
			}
			body.WriteString("\t}\n")
		default:
			return "", fmt.Errorf("experimental lowering: unsupported statement")
		}
	}
	return last, nil
}

// rangeOperand restricts the iteration operand to the documented
// experimental slice: a bare identifier or a recognized navigation chain
// lowers to Go range. Anything else (call, index, compound expression) is
// rejected with a clear error. Type-level checks (slice/array vs map, and
// iteration order) belong to the future typed slice — map iteration order is
// outside the Anuy slice (spec 1-3-1-1 lowering plan, PLAN risk).
func rangeOperand(value parser.Value) (string, error) {
	if value.Navigation != nil {
		return value.Text, nil
	}
	if len(value.Idents) == 1 && value.Idents[0] == value.Text {
		return value.Text, nil
	}
	return "", fmt.Errorf("experimental lowering: unsupported range operand %q", value.Text)
}

// lowerValue renders one right-hand side value; closure literals are emitted
// as Go func literals.
func lowerValue(value parser.Value) (string, error) {
	if value.Closure == nil {
		return value.Text, nil
	}
	cl := value.Closure
	var b strings.Builder
	b.WriteString("func(")
	for i, p := range cl.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		if strings.Contains(p.Type, "?") {
			return "", fmt.Errorf("experimental lowering: unsupported nullable type %q", p.Type)
		}
		b.WriteString(p.Name + " " + p.Type)
	}
	b.WriteString(") {\n")
	if _, err := lowerStatements(&b, cl.Body); err != nil {
		return "", err
	}
	b.WriteString("}")
	return b.String(), nil
}
