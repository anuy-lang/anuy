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
	var last string
	for _, statement := range program.Statements {
		names := strings.Join(statement.Names, ", ")
		switch statement.Kind {
		case parser.Var:
			if statement.Type != "" && strings.Contains(statement.Type, "?") {
				return "", fmt.Errorf("experimental lowering: unsupported nullable type %q", statement.Type)
			}
			switch {
			case statement.Type != "" && len(statement.Values) == 0:
				fmt.Fprintf(&body, "\tvar %s %s\n", names, statement.Type)
			case statement.Type != "":
				fmt.Fprintf(&body, "\tvar %s %s = %s\n", names, statement.Type, strings.Join(statement.Values, ", "))
			default:
				fmt.Fprintf(&body, "\t%s := %s\n", names, strings.Join(statement.Values, ", "))
			}
			last = statement.Names[len(statement.Names)-1]
		case parser.Assign:
			fmt.Fprintf(&body, "\t%s = %s\n", names, strings.Join(statement.Values, ", "))
			last = statement.Names[len(statement.Names)-1]
		case parser.Read:
			fmt.Fprintf(&body, "\t_ = %s\n", statement.Names[0])
			last = statement.Names[0]
		default:
			return "", fmt.Errorf("experimental lowering: unsupported statement")
		}
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	return "package fixture\n\nfunc Run() {\n" + body.String() + "}\n", nil
}
