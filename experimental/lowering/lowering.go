package lowering

import (
	"fmt"
	"strings"

	"github.com/anuy-lang/anuy/experimental/parser"
)

// anuyabiImport is the generated import line for programs that use the
// tagged carrier (RFC-009 §6.1.5 two-contracts sketch): generated Go
// depends on the ABI support package instead of inlining it. Programs
// without tagged representations emit no dependency at all.
const anuyabiImport = "import \"github.com/anuy-lang/anuy/experimental/anuyabi\"\n\n"

// Lower emits deliberately minimal experimental Go for accepted narrow syntax.
func Lower(source string) (string, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return "", err
	}
	l := &lowerer{carriers: map[string]string{}, nativeNil: map[string]bool{}}
	var body strings.Builder
	last, err := l.statements(&body, program.Statements)
	if err != nil {
		return "", err
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	var out strings.Builder
	out.WriteString("package fixture\n\n")
	if l.taggedUsed {
		out.WriteString(anuyabiImport)
	}
	out.WriteString("func Run() {\n")
	out.WriteString(body.String())
	out.WriteString("}\n")
	return out.String(), nil
}

// lowerer carries the per-program state of one lowering run: the names
// declared with a tagged `T?` (their carrier element type drives the
// None/copy conversions of later assignments), the names declared with a
// native-nil nullable shape (story 13 dispatch tracking - `*T?`, `(map…)?`,
// `error?`) and whether the generated file needs the carrier prelude.
type lowerer struct {
	carriers   map[string]string
	nativeNil  map[string]bool
	taggedUsed bool
}

func (l *lowerer) statements(body *strings.Builder, statements []parser.Statement) (string, error) {
	var last string
	for _, statement := range statements {
		names := strings.Join(statement.Names, ", ")
		switch statement.Kind {
		case parser.Var:
			typeText := ""
			var carrierElem string
			if statement.TypeExpr != nil {
				text, err := l.goType(statement.TypeExpr)
				if err != nil {
					return "", err
				}
				typeText = text
				if elem, ok := l.carrier(statement.TypeExpr); ok {
					carrierElem = elem
					for _, name := range statement.Names {
						if name != "_" {
							l.carriers[name] = elem
						}
					}
				} else if statement.TypeExpr.Nullable {
					// Story 13: native-nil declarations enter the dispatch
					// tracking with their verbatim Go comparison form.
					for _, name := range statement.Names {
						if name != "_" {
							l.nativeNil[name] = true
						}
					}
				}
			}
			switch {
			case typeText != "" && len(statement.Values) == 0:
				fmt.Fprintf(body, "\tvar %s %s\n", names, typeText)
			case typeText != "":
				converted := make([]string, 0, len(statement.Values))
				for _, value := range statement.Values {
					text, err := l.convert(value, carrierElem)
					if err != nil {
						return "", err
					}
					converted = append(converted, text)
				}
				fmt.Fprintf(body, "\tvar %s %s = %s\n", names, typeText, strings.Join(converted, ", "))
			default:
				values := make([]string, 0, len(statement.Values))
				for _, value := range statement.Values {
					text, err := l.value(value)
					if err != nil {
						return "", err
					}
					values = append(values, text)
				}
				fmt.Fprintf(body, "\t%s := %s\n", names, strings.Join(values, ", "))
			}
			last = statement.Names[len(statement.Names)-1]
		case parser.Assign:
			// Pairing by index: the arity model of the parser guarantees
			// equal counts except the single-value multi-target form, whose
			// tuple RHS is not modeled - those stay raw text.
			paired := len(statement.Values) == len(statement.Names)
			converted := make([]string, 0, len(statement.Values))
			for i, value := range statement.Values {
				elem := ""
				if paired && statement.Names[i] != "_" {
					elem = l.carriers[statement.Names[i]]
				}
				text, err := l.convert(value, elem)
				if err != nil {
					return "", err
				}
				converted = append(converted, text)
			}
			fmt.Fprintf(body, "\t%s = %s\n", names, strings.Join(converted, ", "))
			last = statement.Names[len(statement.Names)-1]
		case parser.Read:
			fmt.Fprintf(body, "\t_ = %s\n", statement.Names[0])
			last = statement.Names[0]
		case parser.If:
			cond := l.condition(statement.Cond)
			fmt.Fprintf(body, "\tif %s {\n", cond)
			if _, err := l.statements(body, statement.Body); err != nil {
				return "", err
			}
			if statement.Else != nil {
				body.WriteString("\t} else {\n")
				if _, err := l.statements(body, statement.Else); err != nil {
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
				if _, err := l.statements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			case statement.Cond != "":
				fmt.Fprintf(body, "\tfor %s {\n", l.condition(statement.Cond))
				if _, err := l.statements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			default:
				body.WriteString("\tfor {\n")
				if _, err := l.statements(body, statement.Body); err != nil {
					return "", err
				}
				body.WriteString("\t}\n")
			}
		case parser.Call:
			// Story 05: the effectful statement form - the call value is
			// discarded by statement semantics, so no blank discard is
			// appended and nothing feeds the trailing `_ =` line.
			if err := l.callStatement(body, statement.Call); err != nil {
				return "", err
			}
			last = ""
		case parser.Break:
			body.WriteString("\tbreak\n")
		case parser.Continue:
			body.WriteString("\tcontinue\n")
		case parser.Block:
			body.WriteString("\t{\n")
			if _, err := l.statements(body, statement.Body); err != nil {
				return "", err
			}
			body.WriteString("\t}\n")
		default:
			return "", fmt.Errorf("experimental lowering: unsupported statement")
		}
	}
	return last, nil
}

// condition rewrites a nil comparison over a tracked carrier binding to
// the dispatch form (story 13, RFC-002 §6.10.2 - the exact generated form
// is pinned by tests): the verbatim struct comparison does not compile in
// Go. Native-nil operands keep the plain comparison (nil is semantic nil
// there, §6.8.1) and untracked operands keep the source text (no
// information - the Go semantics stand as written).
func (l *lowerer) condition(cond string) string {
	name, ok := nilCompareOperand(cond)
	if !ok {
		return cond
	}
	if _, tracked := l.carriers[name]; tracked {
		return "!" + name + ".IsNil()"
	}
	return cond
}

// nilCompareOperand reports the binding name of the exact `X != nil`
// condition shape (the only nil-comparison form in the experimental slice,
// mirroring the integration narrowing target). Anything else - compound,
// `== nil`, navigation operands - reports no.
func nilCompareOperand(cond string) (string, bool) {
	parts := strings.SplitN(cond, "!=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return "", false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" || strings.ContainsAny(name, " \t.()[]?*") {
		return "", false
	}
	return name, true
}

// callStatement lowers the call statement. Ordinary chains emit verbatim
// (story 05). A safe-tail call `u?.m()` (story 13, RFC-002 §6.4.3, §6.10.4)
// lowers to the synthetic nil-guard branch in the tracked dispatch form of
// the receiver: `!u.IsNil()` plus the member on `u.Value` for the tagged
// carrier, the plain Go `!= nil` guard for native-nil shapes. §6.10.3
// single evaluation is structural on this surface - the grammar admits
// only binding receivers - so no synthetic temporary is introduced.
// A safe segment beyond the receiver has no representation model (fields
// are outside the slice), and an untracked receiver carries no nullable
// information: both reject instead of dropping the `?` - verbatim output
// would panic on an absent value.
func (l *lowerer) callStatement(body *strings.Builder, call *parser.NavigationExpr) error {
	if len(call.Segments) == 1 && call.Segments[0].Safe {
		name, member := call.Receiver, call.Segments[0].Name
		if _, tracked := l.carriers[name]; tracked {
			fmt.Fprintf(body, "\tif !%s.IsNil() {\n\t%s.Value.%s()\n\t}\n", name, name, member)
			return nil
		}
		if l.nativeNil[name] {
			fmt.Fprintf(body, "\tif %s != nil {\n\t%s.%s()\n\t}\n", name, name, member)
			return nil
		}
		return fmt.Errorf("experimental lowering: safe-call receiver %q is not a tracked nullable", name)
	}
	for _, segment := range call.Segments {
		if segment.Safe {
			return fmt.Errorf("experimental lowering: safe-call chain beyond the receiver is not supported")
		}
	}
	expr := call.Receiver
	for _, segment := range call.Segments {
		expr += "." + segment.Name
	}
	fmt.Fprintf(body, "\t%s()\n", expr)
	return nil
}

// goType renders the canonical Go representation of a restricted type
// expression (ADR-0003, RFC-002 §6.7–6.8): value shapes without a free nil
// take the tagged carrier; native-nil shapes keep the plain Go type with nil
// as semantic nil (§6.8.1, §6.8.6, error §6.9.2); plain slices stay
// nil-backed while nullable slices take the carrier (§6.8.9–6.8.13).
func (l *lowerer) goType(t *parser.TypeExpr) (string, error) {
	if t == nil {
		return "", fmt.Errorf("experimental lowering: missing type expression")
	}
	var text string
	switch t.Kind {
	case parser.NamedType:
		text = t.Name
	case parser.PointerType:
		elem, err := l.goType(t.Elem)
		if err != nil {
			return "", err
		}
		text = "*" + elem
	case parser.SliceType:
		elem, err := l.goType(t.Elem)
		if err != nil {
			return "", err
		}
		text = "[]" + elem
	case parser.MapType:
		key, err := l.goType(t.Key)
		if err != nil {
			return "", err
		}
		value, err := l.goType(t.Value)
		if err != nil {
			return "", err
		}
		text = "map[" + key + "]" + value
	default:
		return "", fmt.Errorf("experimental lowering: unsupported type kind %d", t.Kind)
	}
	if !t.Nullable {
		return text, nil
	}
	if (t.Kind == parser.NamedType && t.Name == "error") ||
		t.Kind == parser.PointerType || t.Kind == parser.MapType {
		return text, nil
	}
	l.taggedUsed = true
	return "anuyabi.Nullable[" + text + "]", nil
}

// carrier reports the tagged-carrier element type of a declared type: the
// outermost nullability decides the whole-value representation, so a
// composite spelling with inner nullables (`[](User?)`) passes values
// through unchanged. The restricted grammar cannot spell chan, function or
// interface shapes, and `error` is the one named native-nil type.
func (l *lowerer) carrier(t *parser.TypeExpr) (string, bool) {
	if t == nil || !t.Nullable {
		return "", false
	}
	if (t.Kind == parser.NamedType && t.Name == "error") ||
		t.Kind == parser.PointerType || t.Kind == parser.MapType {
		return "", false
	}
	text, err := l.goType(t)
	if err != nil {
		return "", false
	}
	return strings.TrimSuffix(strings.TrimPrefix(text, "anuyabi.Nullable["), "]"), true
}

// convert renders one right-hand side value for a target declared with the
// carrier element type carrierElem: the nil literal becomes None, a copy of
// an equally-tagged binding passes through (it already holds the carrier
// representation), everything else wraps in Some. Untargeted values and
// native-nil shapes pass through unchanged.
func (l *lowerer) convert(value parser.Value, carrierElem string) (string, error) {
	text, err := l.value(value)
	if err != nil {
		return "", err
	}
	if carrierElem == "" || value.Closure != nil {
		return text, nil
	}
	if value.Text == "nil" {
		l.taggedUsed = true
		return "anuyabi.None[" + carrierElem + "]()", nil
	}
	if len(value.Idents) == 1 && value.Text == value.Idents[0] {
		if _, tagged := l.carriers[value.Idents[0]]; tagged {
			return text, nil
		}
	}
	l.taggedUsed = true
	return "anuyabi.Some(" + text + ")", nil
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

// value renders one right-hand side value; closure literals are emitted
// as Go func literals whose declared parameter types carry their canonical
// representation.
func (l *lowerer) value(value parser.Value) (string, error) {
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
		typeText := p.Type
		if p.TypeExpr != nil {
			text, err := l.goType(p.TypeExpr)
			if err != nil {
				return "", err
			}
			typeText = text
		}
		b.WriteString(p.Name + " " + typeText)
	}
	b.WriteString(") {\n")
	if _, err := l.statements(&b, cl.Body); err != nil {
		return "", err
	}
	b.WriteString("}")
	return b.String(), nil
}
