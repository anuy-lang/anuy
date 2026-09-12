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
		switch statement.Kind {
		case parser.Var:
			fmt.Fprintf(&body, "\tvar %s any\n", statement.Name)
			last = statement.Name
		case parser.Assign:
			fmt.Fprintf(&body, "\t%s = 1\n", statement.Name)
			last = statement.Name
		case parser.Read:
			fmt.Fprintf(&body, "\t_ = %s\n", statement.Name)
			last = statement.Name
		default:
			return "", fmt.Errorf("experimental lowering: unsupported statement")
		}
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	return "package fixture\n\nfunc Run() {\n" + body.String() + "}\n", nil
}
