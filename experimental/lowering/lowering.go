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
	l := &lowerer{carriers: map[string]string{}, nativeNil: map[string]bool{}, interfaces: map[string]bool{}}
	// Story 16: top-level functions hoist to package-level declarations.
	// The Run body lowers first, so the tracking maps are populated by the
	// time the hoisted function bodies are rendered (captures stay
	// visible); output order stays functions-first.
	var bodyStatements, funcs, types []parser.Statement
	for _, statement := range program.Statements {
		switch statement.Kind {
		case parser.Function:
			funcs = append(funcs, statement)
		case parser.TypeDecl, parser.Interface:
			types = append(types, statement)
			if statement.Interface != nil {
				// Story 40 (RFC-009 §6.2.7): interface names pre-register
				// before any body lowers - the carrier() native-nil check
				// consults them regardless of the render order.
				l.interfaces[statement.Interface.Name] = true
			}
		case parser.Impl:
			// Story 39 (RFC-004 §6.8.2): impl is semantic metadata -
			// after checking it erases from the generated Go.
		default:
			bodyStatements = append(bodyStatements, statement)
		}
	}
	var body strings.Builder
	last, err := l.statements(&body, bodyStatements)
	if err != nil {
		return "", err
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	var decls strings.Builder
	for i := range types {
		if err := l.typeDecl(&decls, &types[i]); err != nil {
			return "", err
		}
	}
	for i := range funcs {
		if err := l.function(&decls, &funcs[i]); err != nil {
			return "", err
		}
	}
	var out strings.Builder
	out.WriteString("package fixture\n\n")
	if l.taggedUsed {
		out.WriteString(anuyabiImport)
	}
	out.WriteString(decls.String())
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
	interfaces map[string]bool
	taggedUsed bool
	// returnCarrierElem is the conversion context of the enclosing
	// function's declared result (story 16): the carrier element when the
	// result is tagged, empty for native-nil and non-null results (raw
	// returns). Save/restored around each body.
	returnCarrierElem string
	// returnTypes/returnElems are the per-position result contexts of a
	// result-list declaration (story 35, RFC-005 §6.2.3): the Go type per
	// position drives the failure padding slots (RFC-009 §6.6.3–6.6.5),
	// the carrier element the success-return conversions (empty = raw).
	// Both save/restored around each body.
	returnTypes []string
	returnElems []string
	// temps mints fresh synthetic names (`__anuy_errN`, `__anuy_vN`,
	// `__anuy_padN`) for try temporaries and padding slots - collision
	// free across one program (RFC-009 §6.6.6 naming).
	temps int
}

func (l *lowerer) statements(body *strings.Builder, statements []parser.Statement) (string, error) {
	var last string
	for _, statement := range statements {
		names := strings.Join(statement.Names, ", ")
		switch statement.Kind {
		case parser.Var:
			if statement.TryCall != nil {
				// Story 35 (RFC-005 §6.5.1–6.5.2, RFC-009 §6.6.6): the
				// value-try declaration - call temporaries, the immediate
				// propagation branch, then the plain binding of each
				// success value (definitely initialized on the success
				// path, §6.9.2).
				if err := l.tryDecl(body, &statement); err != nil {
					return "", err
				}
				last = ""
				continue
			}
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
			case len(statement.Values) == 1 && statement.Values[0].Switch != nil:
				// Story 32 (RFC-006 §6.4.4): a value-producing switch
				// requires the declared result type - Go has no switch
				// expressions.
				if typeText == "" || len(statement.Names) != 1 {
					return "", fmt.Errorf("experimental lowering: value switch requires a declared result type")
				}
				fmt.Fprintf(body, "\tvar %s %s\n", statement.Names[0], typeText)
				if err := l.emitSwitchAssignments(body, statement.Names[0], &statement.Values[0], carrierElem); err != nil {
					return "", err
				}
			case typeText != "" && len(statement.Values) == 0:
				fmt.Fprintf(body, "\tvar %s %s\n", names, typeText)
			case typeText != "" && len(statement.Values) == 1 && hasSafeTailValue(statement.Values[0]):
				// Story 14 (RFC-002 §6.10.2): a safe-tail initializer with a
				// declared nullable type lowers to the declaration plus the
				// synthetic guard branch.
				if err := l.safeValueDecl(body, &statement, carrierElem, typeText); err != nil {
					return "", err
				}
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
				// An inferred target has no carrier element to spell
				// None[elem]() with - a safe-tail initializer rejects.
				if len(statement.Values) == 1 && hasSafeTailValue(statement.Values[0]) {
					return "", fmt.Errorf("experimental lowering: safe-value target %q is not a declared nullable", statement.Names[0])
				}
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
			if statement.Target != nil {
				// Story 22: field mutation lowers verbatim (§6.13) - the
				// type-free layer performs no field-type conversions.
				if err := l.fieldMutation(body, &statement); err != nil {
					return "", err
				}
				last = ""
				break
			}
			// Pairing by index: the arity model of the parser guarantees
			// equal counts except the single-value multi-target form, whose
			// tuple RHS is not modeled - those stay raw text.
			paired := len(statement.Values) == len(statement.Names)
			hasSafe := false
			for _, value := range statement.Values {
				if hasSafeTailValue(value) {
					hasSafe = true
					break
				}
			}
			if hasSafe {
				// Story 14: a safe-tail value lowers to its guard branch as a
				// statement of its own; plain siblings emit as single
				// assignments in source order (the joint tuple line cannot
				// interleave with guards). Without pairing or a declared
				// nullable target the None[elem]()/nil branch cannot be
				// spelled - reject.
				if !paired {
					return "", fmt.Errorf("experimental lowering: safe-value assignment is unpaired")
				}
				for i, value := range statement.Values {
					name := statement.Names[i]
					elem := l.carriers[name]
					native := l.nativeNil[name]
					if hasSafeTailValue(value) {
						if name == "_" || (elem == "" && !native) {
							return "", fmt.Errorf("experimental lowering: safe-value target %q is not a declared nullable", name)
						}
						if err := l.safeValue(body, value.Navigation, value.Text, name, elem); err != nil {
							return "", err
						}
						continue
					}
					text, err := l.convert(value, elem)
					if err != nil {
						return "", err
					}
					fmt.Fprintf(body, "\t%s = %s\n", name, text)
				}
				last = statement.Names[len(statement.Names)-1]
				break
			}
			if paired && len(statement.Values) == 1 && statement.Values[0].Switch != nil {
				// Story 32 (RFC-006 §6.4.4): the declared-target form; the
				// carrier element drives the widening conversions.
				if err := l.emitSwitchAssignments(body, statement.Names[0], &statement.Values[0], l.carriers[statement.Names[0]]); err != nil {
					return "", err
				}
				last = statement.Names[0]
				break
			}
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
		case parser.Switch:
			// Story 31 (RFC-006 §6.3, RFC-009 §6.4): the exhaustive enum
			// switch lowers to an ordinary Go switch; case labels are the
			// §6.4.5 constant names (`Color.Red` -> `ColorRed`).
			sw := statement.Switch
			if l.carriers[sw.Scrutinee] != "" {
				// Story 33 (RFC-006 §6.5, §6.11.3): a nullable-enum
				// scrutinee lowers to the representation-specific nullable
				// test plus the Value dispatch; the synthetic default
				// guards the foreign boundary (RFC-009 §6.4.3).
				fmt.Fprintf(body, "\tswitch {\n")
				for _, arm := range sw.Arms {
					if arm.NilArm {
						fmt.Fprintf(body, "\tcase %s.IsNil():\n", sw.Scrutinee)
					} else {
						fmt.Fprintf(body, "\tcase %s.Value == %s%s:\n", sw.Scrutinee, arm.Receiver, arm.Variant)
					}
					if _, err := l.statements(body, arm.Body); err != nil {
						return "", err
					}
				}
				fmt.Fprintf(body, "\tdefault:\n\t\tpanic(\"anuy: invalid enum discriminant\")\n")
				body.WriteString("\t}\n")
				break
			}
			fmt.Fprintf(body, "\tswitch %s {\n", sw.Scrutinee)
			for _, arm := range sw.Arms {
				fmt.Fprintf(body, "\tcase %s%s:\n", arm.Receiver, arm.Variant)
				if _, err := l.statements(body, arm.Body); err != nil {
					return "", err
				}
			}
			body.WriteString("\t}\n")
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
				// iteration, matching §76/§77. A safe-tail collection has no
				// defined iteration semantics yet - it rejects instead of
				// emitting range u?.items.
				if hasSafeTailValue(statement.Values[0]) {
					return "", fmt.Errorf("experimental lowering: safe-tail range operand is not supported")
				}
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
			if err := l.callStatement(body, statement.Call, statement.Values); err != nil {
				return "", err
			}
			last = ""
		case parser.Function:
			// Top-level declarations are hoisted by Lower before the body
			// pass; a Function seen here is block-nested, which has no Go
			// shape (kernel accepts it - a narrowing reject).
			return "", fmt.Errorf("experimental lowering: nested function declaration is not supported")
		case parser.TypeDecl:
			// Top-level declarations are hoisted by Lower before the body
			// pass; a TypeDecl seen here is block-nested, which has no Go
			// shape (kernel accepts it - a narrowing reject).
			return "", fmt.Errorf("experimental lowering: nested type declaration is not supported")
		case parser.Try:
			// Story 34 (RFC-005 §6.5.3, RFC-009 §6.6.6): error-only try -
			// the call plus immediate propagation; the if-initializer
			// temporary preserves exactly-once evaluation (§6.5.8).
			nav := statement.Call
			args := make([]string, 0, len(statement.Values))
			for _, value := range statement.Values {
				text, err := l.value(value)
				if err != nil {
					return "", err
				}
				args = append(args, text)
			}
			expr := nav.Receiver
			for _, segment := range nav.Segments {
				expr += "." + segment.Name
			}
			fmt.Fprintf(body, "\tif err := %s(%s); err != nil {\n\t\treturn err\n\t}\n", expr, strings.Join(args, ", "))
			last = ""
		case parser.Return:
			// Story 16: `return expr` exists only inside a function body
			// with a declared result (the parser funcStack); the conversion
			// follows the result representation. A bare return stays
			// verbatim in every shape. Story 35 (RFC-005 §6.4.2,
			// RFC-009 §6.6.3–6.6.5): the failure form in a strict fallible
			// function materializes one padding slot per success result;
			// story 35 §6.4.1 adds the multi-value success return with
			// per-position conversions.
			if len(statement.Values) == 0 {
				body.WriteString("\treturn\n")
			} else if statement.ErrorReturn && len(l.returnTypes) > 1 {
				operand, err := l.value(statement.Values[0])
				if err != nil {
					return "", err
				}
				pads := make([]string, 0, len(l.returnTypes)-1)
				for i := 0; i < len(l.returnTypes)-1; i++ {
					pad := fmt.Sprintf("__anuy_pad%d", l.temps)
					l.temps++
					fmt.Fprintf(body, "\tvar %s %s\n", pad, l.returnTypes[i])
					pads = append(pads, pad)
				}
				fmt.Fprintf(body, "\treturn %s, %s\n", strings.Join(pads, ", "), operand)
			} else if len(l.returnElems) > 1 {
				parts := make([]string, 0, len(statement.Values))
				for i, value := range statement.Values {
					elem := ""
					if i < len(l.returnElems) {
						elem = l.returnElems[i]
					}
					text, err := l.convert(value, elem)
					if err != nil {
						return "", err
					}
					parts = append(parts, text)
				}
				fmt.Fprintf(body, "\treturn %s\n", strings.Join(parts, ", "))
			} else {
				text, err := l.convert(statement.Values[0], l.returnCarrierElem)
				if err != nil {
					return "", err
				}
				fmt.Fprintf(body, "\treturn %s\n", text)
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

// tryDecl lowers the value-try declaration `var v = try F(x)` (story 35,
// RFC-005 §6.5.1–6.5.2) to the §6.6.6 golden shape: call temporaries, the
// immediate propagation branch (padding slots in a strict fallible
// function, RFC-009 §6.6.3–6.6.5), then the plain binding of each success
// value. Exactly-once evaluation survives through the temporaries
// (§6.6.7); the synthetic `__anuy_` names cannot collide with source
// bindings.
func (l *lowerer) tryDecl(body *strings.Builder, statement *parser.Statement) error {
	call := statement.TryCall
	args := make([]string, 0, len(statement.Values))
	for _, value := range statement.Values {
		text, err := l.value(value)
		if err != nil {
			return err
		}
		args = append(args, text)
	}
	expr := call.Receiver
	for _, segment := range call.Segments {
		expr += "." + segment.Name
	}
	errTmp := fmt.Sprintf("__anuy_err%d", l.temps)
	l.temps++
	vtemps := make([]string, 0, len(statement.Names))
	lhs := make([]string, 0, len(statement.Names)+1)
	for range statement.Names {
		vtmp := fmt.Sprintf("__anuy_v%d", l.temps)
		l.temps++
		vtemps = append(vtemps, vtmp)
		lhs = append(lhs, vtmp)
	}
	lhs = append(lhs, errTmp)
	fmt.Fprintf(body, "\t%s := %s(%s)\n", strings.Join(lhs, ", "), expr, strings.Join(args, ", "))
	if len(l.returnTypes) > 1 {
		// Strict fallible enclosing function: the propagation branch
		// materializes one padding slot per success result.
		body.WriteString("\tif " + errTmp + " != nil {\n")
		pads := make([]string, 0, len(l.returnTypes)-1)
		for i := 0; i < len(l.returnTypes)-1; i++ {
			pad := fmt.Sprintf("__anuy_pad%d", l.temps)
			l.temps++
			fmt.Fprintf(body, "\t\tvar %s %s\n", pad, l.returnTypes[i])
			pads = append(pads, pad)
		}
		fmt.Fprintf(body, "\t\treturn %s, %s\n", strings.Join(pads, ", "), errTmp)
		body.WriteString("\t}\n")
	} else {
		// Error-only enclosing function (§6.5.3): the propagated error is
		// the whole result.
		fmt.Fprintf(body, "\tif %s != nil {\n\t\treturn %s\n\t}\n", errTmp, errTmp)
	}
	for i, name := range statement.Names {
		if name == "_" {
			continue
		}
		fmt.Fprintf(body, "\t%s := %s\n", name, vtemps[i])
	}
	return nil
}

// fieldMutation lowers `u.f = expr` verbatim (story 22, RFC-014 §6.13):
// the type-free layer performs no field-type conversions.
func (l *lowerer) fieldMutation(body *strings.Builder, statement *parser.Statement) error {
	expr := statement.Target.Receiver
	for _, segment := range statement.Target.Segments {
		expr += "." + segment.Name
	}
	if len(statement.Values) == 1 && statement.Values[0].Switch != nil {
		// Story 32 (RFC-006 §6.4.4): a value-producing switch on a field
		// mutation target.
		if err := l.emitSwitchAssignments(body, expr, &statement.Values[0], ""); err != nil {
			return err
		}
		return nil
	}
	texts := make([]string, 0, len(statement.Values))
	for _, value := range statement.Values {
		text, err := l.value(value)
		if err != nil {
			return err
		}
		texts = append(texts, text)
	}
	fmt.Fprintf(body, "\t%s = %s\n", expr, strings.Join(texts, ", "))
	return nil
}

// emitSwitchAssignments writes a Go switch whose arms assign the arm
// values to target (story 32, RFC-006 §6.4.4): the Go emulation of a
// value-producing switch - Go has no switch expressions; carrierElem
// drives the §6.4.4 widening conversions.
func (l *lowerer) emitSwitchAssignments(body *strings.Builder, target string, value *parser.Value, carrierElem string) error {
	sw := value.Switch
	fmt.Fprintf(body, "\tswitch %s {\n", sw.Scrutinee)
	for _, arm := range sw.Arms {
		if arm.Value == nil {
			return fmt.Errorf("experimental lowering: value switch arm requires a value")
		}
		fmt.Fprintf(body, "\tcase %s%s:\n", arm.Receiver, arm.Variant)
		text, err := l.convert(*arm.Value, carrierElem)
		if err != nil {
			return err
		}
		fmt.Fprintf(body, "\t\t%s = %s\n", target, text)
	}
	body.WriteString("\t}\n")
	return nil
}

// condition rewrites nil comparisons over a tracked carrier binding to
// the dispatch form (stories 13–14, RFC-002 §6.10.2 - the exact generated
// form is pinned by tests): verbatim struct comparisons do not compile in
// Go. Native-nil operands keep the plain comparison (nil is semantic nil
// there, §6.8.1) and untracked operands keep the source text (no
// information - the Go semantics stand as written).
func (l *lowerer) condition(cond string) string {
	if name, ok := nilCompareOperand(cond, "!="); ok {
		if _, tracked := l.carriers[name]; tracked {
			return "!" + name + ".IsNil()"
		}
	} else if name, ok := nilCompareOperand(cond, "=="); ok {
		if _, tracked := l.carriers[name]; tracked {
			return name + ".IsNil()"
		}
	}
	return cond
}

// nilCompareOperand reports the binding name of the exact `X != nil` /
// `X == nil` condition shapes (the only nil-comparison forms in the
// experimental slice, mirroring the integration narrowing target).
// Anything else - compound, navigation operands - reports no.
func nilCompareOperand(cond, op string) (string, bool) {
	parts := strings.SplitN(cond, op, 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return "", false
	}
	name := strings.TrimSpace(parts[0])
	if name == "" || strings.ContainsAny(name, " \t.()[]?*") {
		return "", false
	}
	return name, true
}

// callStatement lowers the call statement (story 05). Arguments emit
// through the value renderer (story 15: raw text and func literals;
// they were dropped entirely before). A safe-tail argument rejects -
// argument position has no dispatch target (the story 14 argument).
// Ordinary chains emit verbatim; a safe-tail call `u?.m()` (story 13,
// RFC-002 §6.4.3, §6.10.4) lowers to the synthetic nil-guard branch in
// the tracked dispatch form of the receiver - `!u.IsNil()` plus the
// member on `u.Value` for the tagged carrier, the plain Go `!= nil`
// guard for native-nil shapes - with the arguments inside the branch,
// so they evaluate only when the receiver is present (§6.10.4).
// §6.10.3 single evaluation is structural on this surface - the grammar
// admits only binding receivers - so no synthetic temporary is
// introduced. A safe segment beyond the receiver has no representation
// model (fields are outside the slice), and an untracked receiver
// carries no nullable information: both reject instead of dropping the
// `?` - verbatim output would panic on an absent value.
func (l *lowerer) callStatement(body *strings.Builder, call *parser.NavigationExpr, values []parser.Value) error {
	args := make([]string, 0, len(values))
	for _, value := range values {
		if hasSafeTailValue(value) {
			return fmt.Errorf("experimental lowering: safe-tail call argument is not supported")
		}
		text, err := l.value(value)
		if err != nil {
			return err
		}
		args = append(args, text)
	}
	joined := strings.Join(args, ", ")
	if len(call.Segments) == 1 && call.Segments[0].Safe {
		name, member := call.Receiver, call.Segments[0].Name
		if _, tracked := l.carriers[name]; tracked {
			fmt.Fprintf(body, "\tif !%s.IsNil() {\n\t%s.Value.%s(%s)\n\t}\n", name, name, member, joined)
			return nil
		}
		if l.nativeNil[name] {
			fmt.Fprintf(body, "\tif %s != nil {\n\t%s.%s(%s)\n\t}\n", name, name, member, joined)
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
	fmt.Fprintf(body, "\t%s(%s)\n", expr, joined)
	return nil
}

// hasSafeTailValue reports whether the value carries a safe segment - the
// dispatch trigger of story 14. Ordinary navigation stays verbatim.
func hasSafeTailValue(value parser.Value) bool {
	if value.Navigation == nil {
		return false
	}
	for _, segment := range value.Navigation.Segments {
		if segment.Safe {
			return true
		}
	}
	return false
}

// safeValueDecl lowers a single-value typed declaration whose initializer
// is a safe-tail value: the plain declaration, then the guard branch into
// the target. The blank identifier is not a binding and the target must be
// declared nullable - otherwise the None[elem]()/nil branch cannot be
// spelled.
func (l *lowerer) safeValueDecl(body *strings.Builder, statement *parser.Statement, carrierElem, typeText string) error {
	if statement.Names[0] == "_" || (carrierElem == "" && !statement.TypeExpr.Nullable) {
		return fmt.Errorf("experimental lowering: safe-value target %q is not a declared nullable", statement.Names[0])
	}
	fmt.Fprintf(body, "\tvar %s %s\n", statement.Names[0], typeText)
	return l.safeValue(body, statement.Values[0].Navigation, statement.Values[0].Text, statement.Names[0], carrierElem)
}

// safeValue emits the synthetic guard branch assigning a safe-tail value
// to the target (story 14, RFC-002 §6.4.5, §6.10.2): the guard follows the
// receiver representation (`!x.IsNil()` plus the member on x.Value for the
// carrier, the plain Go comparison plus the member on x for native-nil),
// the conversion follows the target representation (`Some(member)` /
// `None[elem]()` for a carrier target, the raw member / nil for a
// native-nil target). A zero-argument member call emits the call form;
// call arguments are invisible in NavigationExpr, so an argumented call
// rejects. §6.10.3 single evaluation is structural: the dispatch requires
// a tracked binding receiver, and a root call is invisible - hence
// untracked and rejected. Safe segments beyond the receiver have no
// representation model and reject.
func (l *lowerer) safeValue(body *strings.Builder, nav *parser.NavigationExpr, raw, target, carrierElem string) error {
	if len(nav.Segments) != 1 || !nav.Segments[0].Safe {
		return fmt.Errorf("experimental lowering: safe-value chain beyond the receiver is not supported")
	}
	name, member := nav.Receiver, nav.Segments[0].Name
	var guard, memberExpr string
	if _, tracked := l.carriers[name]; tracked {
		guard = "!" + name + ".IsNil()"
		memberExpr = name + ".Value." + member
	} else if l.nativeNil[name] {
		guard = name + " != nil"
		memberExpr = name + "." + member
	} else {
		return fmt.Errorf("experimental lowering: safe-value receiver %q is not a tracked nullable", name)
	}
	if nav.Segments[0].Call {
		if !strings.Contains(raw, member+"()") {
			return fmt.Errorf("experimental lowering: safe-value call arguments are not supported")
		}
		memberExpr += "()"
	}
	var thenValue, elseValue string
	if carrierElem != "" {
		l.taggedUsed = true
		thenValue = "anuyabi.Some(" + memberExpr + ")"
		elseValue = "anuyabi.None[" + carrierElem + "]()"
	} else {
		thenValue, elseValue = memberExpr, "nil"
	}
	fmt.Fprintf(body, "\tif %s {\n\t%s = %s\n\t} else {\n\t%s = %s\n\t}\n", guard, target, thenValue, target, elseValue)
	return nil
}

// methodReceiver is the synthetic receiver name of a lowered method: the
// grammar has no receiver binding (the body never reads it), and verbatim
// calls (`u.save()`) compile only against a real Go method.
const methodReceiver = "anuyRecv"

// function lowers one hoisted top-level declaration (story 16, RFC-002
// §6.4.11-6.4.12, §6.7-6.8): parameters and the declared result render
// through the representation rules, and the body lowers with the return
// context set to the result's carrier element (empty for native-nil and
// non-null results - raw returns). `//anuy:pure` is a kernel contract and
// a no-op for generated Go.
func (l *lowerer) function(decls *strings.Builder, statement *parser.Statement) error {
	cl := statement.Closure
	var b strings.Builder
	b.WriteString("func ")
	if statement.Method != "" {
		// Story 39 (RFC-004 §6.2.1): the pointer-receiver spelling lowers
		// to the Go pointer receiver.
		receiverType := statement.Method
		if statement.MethodPointer {
			receiverType = "*" + receiverType
		}
		b.WriteString("(" + methodReceiver + " " + receiverType + ") ")
	}
	b.WriteString(statement.Names[0] + "(")
	for i, p := range cl.Params {
		if i > 0 {
			b.WriteString(", ")
		}
		typeText := p.Type
		if p.TypeExpr != nil {
			text, err := l.goType(p.TypeExpr)
			if err != nil {
				return err
			}
			typeText = text
		}
		b.WriteString(p.Name + " " + typeText)
	}
	b.WriteString(")")
	if len(cl.ResultList) > 0 {
		// Story 35 (RFC-005 §6.2.3, RFC-009 §6.6.2): the strict fallible
		// Go ABI - one Go result per declared position, trailing `error`.
		parts := make([]string, 0, len(cl.ResultList))
		for _, t := range cl.ResultList {
			text, err := l.goType(t)
			if err != nil {
				return err
			}
			parts = append(parts, text)
		}
		b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	} else if statement.HasResult && cl.ResultTypeExpr != nil {
		text, err := l.goType(cl.ResultTypeExpr)
		if err != nil {
			return err
		}
		b.WriteString(" " + text)
	}
	b.WriteString(" {\n")
	saved := l.returnCarrierElem
	savedTypes, savedElems := l.returnTypes, l.returnElems
	if len(cl.ResultList) > 0 {
		// Story 35: per-position conversion context - the Go type drives
		// the failure padding, the carrier element the success-return
		// conversions (empty string = raw position, e.g. the trailing
		// error).
		for _, t := range cl.ResultList {
			text, err := l.goType(t)
			if err != nil {
				l.returnCarrierElem = saved
				l.returnTypes, l.returnElems = savedTypes, savedElems
				return err
			}
			elem := ""
			if carrierElem, ok := l.carrier(t); ok {
				elem = carrierElem
			}
			l.returnTypes = append(l.returnTypes, text)
			l.returnElems = append(l.returnElems, elem)
		}
	} else if statement.HasResult && cl.ResultTypeExpr != nil {
		if elem, ok := l.carrier(cl.ResultTypeExpr); ok {
			l.returnCarrierElem = elem
		}
	}
	if _, err := l.statements(&b, cl.Body); err != nil {
		l.returnCarrierElem = saved
		l.returnTypes, l.returnElems = savedTypes, savedElems
		return err
	}
	l.returnCarrierElem = saved
	l.returnTypes, l.returnElems = savedTypes, savedElems
	b.WriteString("}\n")
	decls.WriteString(b.String() + "\n")
	return nil
}

// interfaceDecl lowers one interface declaration (story 39, RFC-004
// §6.8.1, RFC-009 §6.5.1): an ordinary Go interface with the equivalent
// lowered method signatures - ordinary Go interface dispatch, no Anuy
// runtime machinery (§6.5.2 no vtable).
func (l *lowerer) interfaceDecl(decls *strings.Builder, statement *parser.Statement) error {
	decl := statement.Interface
	var b strings.Builder
	b.WriteString("type " + decl.Name + " interface {\n")
	for _, method := range decl.Methods {
		parts := make([]string, 0, len(method.Params))
		for _, p := range method.Params {
			typeText := p.Type
			if p.TypeExpr != nil {
				text, err := l.goType(p.TypeExpr)
				if err != nil {
					return err
				}
				typeText = text
			}
			parts = append(parts, p.Name+" "+typeText)
		}
		b.WriteString("\t" + method.Name + "(" + strings.Join(parts, ", ") + ")")
		if method.HasResult && method.ResultTypeExpr != nil {
			text, err := l.goType(method.ResultTypeExpr)
			if err != nil {
				return err
			}
			b.WriteString(" " + text)
		}
		b.WriteString("\n")
	}
	b.WriteString("}\n")
	decls.WriteString(b.String() + "\n")
	return nil
}

// typeDecl lowers one hoisted struct declaration (story 21, RFC-014 §6.2,
// §6.13): an ordinary Go struct with the field order preserved and the
// field types through the representation rules. Construction literals
// lower verbatim (§6.13 - "practically directly"); their completeness is
// a kernel concern (story 22).
func (l *lowerer) typeDecl(decls *strings.Builder, statement *parser.Statement) error {
	if statement.Interface != nil {
		return l.interfaceDecl(decls, statement)
	}
	if statement.Enum != nil {
		return l.enumDecl(decls, statement)
	}
	sd := statement.Struct
	var b strings.Builder
	b.WriteString("type " + sd.Name + " struct {\n")
	for _, field := range sd.Fields {
		text, err := l.goType(field.TypeExpr)
		if err != nil {
			return err
		}
		if field.Embedded {
			// Story 29 (RFC-014 §6.9, normative 63): `embed` lowers to an
			// ordinary Go embedded field - the bare type, no name.
			fmt.Fprintf(&b, "\t%s\n", text)
			continue
		}
		fmt.Fprintf(&b, "\t%s %s\n", field.Name, text)
	}
	b.WriteString("}\n")
	decls.WriteString(b.String() + "\n")
	return nil
}

// enumDecl lowers an enum declaration (story 30, RFC-009 §6.4): a named
// uint32 plus discriminant constants 1..N in declaration order - the Go
// zero value stays the reserved invalid representation (§6.4.2-6.4.3);
// constant names follow §6.4.5 (`Color.Red` -> `ColorRed`).
func (l *lowerer) enumDecl(decls *strings.Builder, statement *parser.Statement) error {
	ed := statement.Enum
	var b strings.Builder
	fmt.Fprintf(&b, "type %s uint32\n\nconst (\n", ed.Name)
	for i, variant := range ed.Variants {
		fmt.Fprintf(&b, "\t%s%s %s = %d\n", ed.Name, variant.Name, ed.Name, i+1)
	}
	b.WriteString(")\n")
	decls.WriteString(b.String() + "\n")
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
	// Native-nil shapes keep the plain Go type with nil as semantic nil
	// (§6.2.3, §6.2.7): `error`, pointer, map and - since story 40 -
	// declared interfaces (`Reader?` lowers to `Reader`).
	if (t.Kind == parser.NamedType && (t.Name == "error" || l.interfaces[t.Name])) ||
		t.Kind == parser.PointerType || t.Kind == parser.MapType {
		return text, nil
	}
	l.taggedUsed = true
	return "anuyabi.Nullable[" + text + "]", nil
}

// carrier reports the tagged-carrier element type of a declared type: the
// outermost nullability decides the whole-value representation, so a
// composite spelling with inner nullables (`[](User?)`) passes values
// through unchanged. Native-nil shapes skip the carrier (§6.2.3, §6.2.7):
// `error`, pointer, map and channel-function shapes the grammar cannot
// spell, and - since story 40 - declared interfaces whose `I?` lowers to
// the plain Go interface with nil as semantic nil.
func (l *lowerer) carrier(t *parser.TypeExpr) (string, bool) {
	if t == nil || !t.Nullable {
		return "", false
	}
	if (t.Kind == parser.NamedType && (t.Name == "error" || l.interfaces[t.Name])) ||
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
