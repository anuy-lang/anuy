package lowering

import (
	"errors"
	"fmt"
	"go/token"
	"sort"
	"strings"

	"github.com/anuy-lang/anuy/experimental/parser"
)

// anuyabiImport is the generated import line for programs that use the
// tagged carrier (RFC-009 §6.1.5 two-contracts sketch): generated Go
// depends on the ABI support package instead of inlining it. Programs
// without tagged representations emit no dependency at all.
const anuyabiImport = "import \"github.com/anuy-lang/anuy/anuyabi\"\n\n"

// Lower emits deliberately minimal experimental Go for accepted narrow syntax.
func Lower(source string) (string, error) {
	// Story 59 (RFC-010 §6.9.8): the pathless entry anchors to a canonical
	// spelling, so one-text pins stay deterministic (§6.2.11 one lowering).
	return LowerFile("source.anuy", source)
}

// LowerFile lowers source, anchoring generated positions to path via
// //line directives (story 59, RFC-010 §6.9.8): the Go toolchain itself
// reports diagnostics against the Anuy source. The path spelling is
// carried verbatim into the generated text (§6.9.3).
func LowerFile(path, source string) (string, error) {
	return LowerPackage([]File{{Path: path, Source: source}})
}

// File is one source file of a package lowering (story 62, RFC-015 §6.1
// v4).
type File struct {
	Path   string
	Source string
}

// importAnchor is one declared import with its first-occurrence anchor
// (story 63, RFC-015 §6.15 v5).
type importAnchor struct {
	path  string
	span  parser.Span
	owner File
}

// filePlan is one file's classification inside the package lowering:
// the statements that render into the package declarations and the Run
// body, in source order.
type filePlan struct {
	file    File
	program parser.Program
	types   []parser.Statement
	funcs   []parser.Statement
	body    []parser.Statement
}

// LowerPackage lowers one package directory's files into a single
// generated file (story 62, §6.2.6 RFC-010: physical placement is not a
// semantic property): all declarations with //line anchors to their own
// sources and one Run whose body concatenates the files' statements in
// the given (deterministic) order - per-file Run bodies would collide in
// one package. The clause is uniform across the files (checked upstream,
// ANUY9002); the first file's name names the package.
func LowerPackage(files []File) (string, error) {
	return lowerPackage(files, false)
}

// LowerTestPackage lowers the test variant of a package (story 67,
// §6.5.2/§6.5.4): the test file's declarations are not published-API
// foreign entries, so the §6.8 boundary wrappers do not apply to them -
// the variant carries no anuyabi dependency of its own.
func LowerTestPackage(files []File) (string, error) {
	return lowerPackage(files, true)
}

func lowerPackage(files []File, testVariant bool) (string, error) {
	if len(files) == 0 {
		return "", errors.New("lowering: no files")
	}
	l := &lowerer{testVariant: testVariant, carriers: map[string]string{}, nativeNil: map[string]bool{}, interfaces: map[string]bool{}, enums: map[string]int{}, wrappedFns: map[string]bool{}, funcResults: map[string]*parser.TypeExpr{}, structs: map[string][]parser.StructField{}, typeSpans: map[string]parser.Span{}, typeFiles: map[string]File{}, validatorsPlanned: map[string]bool{}, hasInvMemo: map[string]bool{}, hasInvWip: map[string]bool{}, seenMemo: map[string]bool{}, seenWip: map[string]bool{}}
	plans := make([]filePlan, len(files))
	for i := range files {
		program, err := parser.Parse(files[i].Source)
		if err != nil {
			return "", err
		}
		plans[i] = filePlan{file: files[i], program: program}
		if err := l.plan(&plans[i]); err != nil {
			return "", err
		}
	}
	// Rendering fills the buffers first - the import gate reads flags
	// (taggedUsed/boundaryUsed) that rendering sets, and the Run bodies
	// lower before the hoisted function bodies so the tracking maps are
	// populated when the captures render (story 16); the output order
	// stays functions-first.
	var decls, funcsBuf, body strings.Builder
	for i := range plans {
		if err := l.renderBody(&body, &plans[i]); err != nil {
			return "", err
		}
	}
	// Import block: sorted, deduplicated union across the files (story
	// 63, RFC-015 §6.15 v5); the first occurrence in file order carries
	// the //line anchor.
	var imports []importAnchor
	seen := make(map[string]bool)
	for i := range plans {
		for _, imp := range plans[i].program.Imports {
			if seen[imp.Path] {
				continue
			}
			seen[imp.Path] = true
			imports = append(imports, importAnchor{imp.Path, imp.Span, plans[i].file})
		}
	}
	sort.Slice(imports, func(a, b int) bool { return imports[a].path < imports[b].path })
	for _, imp := range imports {
		l.path, l.source = imp.owner.Path, imp.owner.Source
		l.directive(&decls, imp.span)
		fmt.Fprintf(&decls, "import %q\n", imp.path)
	}
	for i := range plans {
		if err := l.renderDecls(&decls, &plans[i]); err != nil {
			return "", err
		}
	}
	for _, v := range l.validators {
		// Synthetic declarations anchor to the owning type (§6.9.8) in
		// its own file - not to the physical line they occupy.
		l.path, l.source = v.owner.Path, v.owner.Source
		l.directive(&decls, v.anchor)
		decls.WriteString(v.src)
	}
	for i := range plans {
		if err := l.renderFuncs(&funcsBuf, &plans[i]); err != nil {
			return "", err
		}
	}
	var out strings.Builder
	out.WriteString("package " + plans[0].program.Package + "\n\n")
	if l.taggedUsed || l.boundaryUsed {
		out.WriteString(anuyabiImport)
	}
	out.WriteString(decls.String())
	out.WriteString(funcsBuf.String())
	// The test variant carries no program entry - the Run synthesis is
	// the production program body (story 67, §6.5.2).
	if !testVariant {
		out.WriteString("func Run() {\n")
		out.WriteString(body.String())
		out.WriteString("}\n")
	}
	return out.String(), nil
}

// plan classifies one file's statements and runs the per-package
// planning passes over them (stories 16/40/42/43/49: hoisting,
// pre-registration, wrapper and validator planning).
func (l *lowerer) plan(plan *filePlan) error {
	l.path = plan.file.Path
	l.source = plan.file.Source
	var funcs, types []parser.Statement
	for _, statement := range plan.program.Statements {
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
				l.typeSpans[statement.Interface.Name] = statement.Span
				l.typeFiles[statement.Interface.Name] = plan.file
			}
			if statement.Enum != nil {
				// Story 42 (RFC-009 §6.8.6): the variant count pins the
				// contiguous discriminant range of the boundary check.
				l.enums[statement.Enum.Name] = len(statement.Enum.Variants)
				l.typeSpans[statement.Enum.Name] = statement.Span
				l.typeFiles[statement.Enum.Name] = plan.file
			}
			if statement.Struct != nil {
				// Story 43 (RFC-009 §6.8.16): the declared fields are the
				// type-graph input of the aggregate boundary analysis.
				l.structs[statement.Struct.Name] = statement.Struct.Fields
				l.typeSpans[statement.Struct.Name] = statement.Span
				l.typeFiles[statement.Struct.Name] = plan.file
			}
		case parser.Impl:
			// Story 39 (RFC-004 §6.8.2): impl is semantic metadata -
			// after checking it erases from the generated Go.
		default:
			plan.body = append(plan.body, statement)
		}
	}
	plan.types, plan.funcs = types, funcs
	// Story 42 (RFC-009 §6.8): plan the boundary wrappers before any body
	// renders - direct calls retarget to the native entry (§6.8.14)
	// regardless of declaration order. The test variant (story 67) plans
	// none: test declarations are not published-API foreign entries.
	if l.testVariant {
		return l.planValidators(funcs)
	}
	for i := range funcs {
		statement := funcs[i]
		name := statement.Names[0]
		if !token.IsExported(name) {
			continue
		}
		checks, _, err := l.boundaryChecks(&statement)
		if err != nil {
			return err
		}
		if len(checks) > 0 {
			l.wrappedFns[name] = true
		}
	}
	// Story 49 (RFC-009 §6.7.13): collect the declared first results of
	// the hoisted top-level functions - the inference table of the
	// inferred `var x = f()` dispatch registration - before any body
	// renders, so declaration order is not observable.
	for i := range funcs {
		statement := funcs[i]
		if statement.Closure == nil {
			continue
		}
		var result *parser.TypeExpr
		if len(statement.Closure.ResultList) > 0 {
			result = statement.Closure.ResultList[0]
		} else if statement.HasResult {
			result = statement.Closure.ResultTypeExpr
		}
		l.funcResults[statement.Names[0]] = result
	}
	// Story 43 (RFC-009 §6.8.16): plan the per-type validators from the
	// exported signatures - before any body renders, so wrapper and
	// validator emission share one settled analysis.
	return l.planValidators(funcs)
}

// renderDecls renders the file's type declarations with its //line
// anchors.
func (l *lowerer) renderDecls(out *strings.Builder, plan *filePlan) error {
	l.path, l.source = plan.file.Path, plan.file.Source
	for i := range plan.types {
		l.directive(out, plan.types[i].Span)
		if err := l.typeDecl(out, &plan.types[i]); err != nil {
			return err
		}
	}
	return nil
}

// renderFuncs renders the file's functions with its //line anchors: one
// directive per declaration - the foreign-entry wrapper (story 42) and
// the `__anuy_` native entry share the owner's anchor.
func (l *lowerer) renderFuncs(out *strings.Builder, plan *filePlan) error {
	l.path, l.source = plan.file.Path, plan.file.Source
	for i := range plan.funcs {
		l.directive(out, plan.funcs[i].Span)
		if err := l.function(out, &plan.funcs[i]); err != nil {
			return err
		}
	}
	return nil
}

// renderBody renders the file's Run-body statements with its //line
// anchors.
func (l *lowerer) renderBody(out *strings.Builder, plan *filePlan) error {
	l.path, l.source = plan.file.Path, plan.file.Source
	var body strings.Builder
	last, err := l.statements(&body, plan.body)
	if err != nil {
		return err
	}
	if last != "" && !strings.Contains(body.String(), "_ = ") {
		fmt.Fprintf(&body, "\t_ = %s\n", last)
	}
	out.WriteString(body.String())
	return nil
}

// lowerer carries the per-program state of one lowering run: the names
// declared with a tagged `T?` (their carrier element type drives the
// None/copy conversions of later assignments), the names declared with a
// native-nil nullable shape (story 13 dispatch tracking - `*T?`, `(map…)?`,
// `error?`) and whether the generated file needs the carrier prelude.
type lowerer struct {
	// path/source carry the //line anchor inputs (story 59, RFC-010
	// §6.9.8): path verbatim as passed by the caller (§6.9.3), source for
	// the offset→line conversion of statement spans.
	path   string
	source string
	// testVariant marks the test-variant lowering (story 67, §6.5.2):
	// its exported declarations are not published-API foreign entries -
	// §6.8 boundary wrappers do not plan for them.
	testVariant bool
	// typeSpans maps a declared type name to its declaration span - the
	// anchor of the synthetic validators derived from that type (§6.9.8);
	// typeFiles carries the owning file of that anchor (story 62).
	typeSpans  map[string]parser.Span
	typeFiles  map[string]File
	carriers   map[string]string
	nativeNil  map[string]bool
	interfaces map[string]bool
	// enums maps a declared enum name to its variant count (story 42,
	// RFC-009 §6.8.6): the boundary range check pins 1..variants.
	enums map[string]int
	// structs maps a declared struct name to its declared fields (story
	// 43, RFC-009 §6.8.16): the type-graph input of the boundary
	// analysis - embedded fields keep their derived name and flag.
	structs map[string][]parser.StructField
	// hasInvMemo/hasInvWip memoize the transitive invariant predicate
	// (§6.8.15): in-progress marks the fixpoint cutoff - a type reachable
	// only through itself contributes nothing.
	hasInvMemo map[string]bool
	hasInvWip  map[string]bool
	// seenMemo/seenWip memoize the seen-map requirement (§6.8.9, §6.8.17):
	// the type reaches a pointer cycle through its field closure.
	seenMemo map[string]bool
	seenWip  map[string]bool
	// validators holds the emitted per-type validator sources (§6.8.16),
	// lazily planned and deterministically sorted (§13 rule 76), with the
	// owning type's span as the //line anchor (story 59, §6.9.8);
	// validatorsPlanned guards against per-file planning re-appending a
	// shared type's validator (story 62).
	validators        []anchored
	validatorsPlanned map[string]bool
	// wrappedFns holds the exported top-level functions that receive a
	// foreign-entry wrapper (story 42, RFC-009 §6.8): direct generated
	// calls to them retarget to the `__anuy_` native entry (§6.8.14).
	// Wrapped methods stay out - method calls keep the wrapper in this
	// slice (identical semantics, §6.8.10 cost).
	wrappedFns map[string]bool
	// funcResults holds the declared first result of each hoisted
	// top-level function (story 49, RFC-009 §6.7.13): the inference
	// table of the inferred `var x = f()` dispatch registration - built
	// before any body renders, so declaration order is not observable.
	funcResults map[string]*parser.TypeExpr
	// boundaryUsed records whether any wrapper emitted an anuyabi.Require*
	// check - the generated file then imports the support package even
	// without tagged carriers.
	boundaryUsed bool
	taggedUsed   bool
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

// anchored pairs a synthetic declaration with the source span it anchors
// to (story 59, RFC-010 §6.9.8).
type anchored struct {
	src    string
	anchor parser.Span
	// owner is the file of the type the synthetic declaration derives
	// from - the //line anchor source (story 62).
	owner File
}

// directive anchors the next generated line to the construct's source
// line (story 59, RFC-010 §6.9.8). Go honors //line only at the start of
// a line, so the emission is always at column zero; zero spans (fully
// synthetic statements) stay unanchored.
func (l *lowerer) directive(w *strings.Builder, span parser.Span) {
	if l.path == "" || span.End <= 0 {
		return
	}
	fmt.Fprintf(w, "//line %s:%d\n", l.path, lineOf(l.source, span.Start))
}

// lineOf converts a byte offset to its 1-based source line.
func lineOf(source string, offset int) int {
	line := 1
	for i := 0; i < offset && i < len(source); i++ {
		if source[i] == '\n' {
			line++
		}
	}
	return line
}

func (l *lowerer) statements(body *strings.Builder, statements []parser.Statement) (string, error) {
	var last string
	for _, statement := range statements {
		l.directive(body, statement.Span)
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
			// Story 49 (RFC-009 §6.7.13): an inferred declaration takes its
			// dispatch representation from the initializer's declared first
			// result - a single bare call to a known top-level function.
			// Unknown callees stay untracked: the plain comparison stands
			// where Go semantics do (§6.2.2), and guard-demanding forms keep
			// rejecting (§6.7.5).
			if typeText == "" && len(statement.Values) == 1 && len(statement.Names) == 1 && statement.Names[0] != "_" {
				if callee, ok := plainCallCallee(statement.Values[0]); ok {
					if result := l.funcResults[callee]; result != nil && result.Nullable {
						if elem, ok := l.carrier(result); ok {
							l.carriers[statement.Names[0]] = elem
						} else {
							l.nativeNil[statement.Names[0]] = true
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
			if statement.Call.Receiver == "assume_non_nil" {
				// Story 41 (RFC-007 §6.6.2/§6.6.6): the intrinsic is not a
				// callee - it erases to an ordinary read of its operand.
				if len(statement.Values) == 1 {
					body.WriteString("\t_ = " + statement.Values[0].Text + "\n")
				}
				last = ""
				continue
			}
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
		case parser.UnsafeBlock:
			// Story 41 (RFC-007 §6.6.2): a compile-time permission
			// context - no runtime mode, no frame, nothing to emit.
			if _, err := l.statements(body, statement.Body); err != nil {
				return "", err
			}
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
	// Story 42 (RFC-009 §6.8.14): the try call follows the same native
	// entry retargeting as ordinary call statements.
	if len(call.Segments) == 0 && l.wrappedFns[call.Receiver] {
		expr = "__anuy_" + expr
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

// plainCallCallee reports the callee of a value that is exactly a bare
// call `name(...)` - the same shape the §6.8.14 native-entry retarget
// matches. Selector chains, closures, keyed literals and value switches
// report no.
func plainCallCallee(value parser.Value) (string, bool) {
	if value.Navigation != nil || value.Switch != nil || value.Keyed != nil || value.Closure != nil {
		return "", false
	}
	if len(value.Idents) != 1 {
		return "", false
	}
	name := value.Idents[0]
	if !strings.HasPrefix(value.Text, name+"(") {
		return "", false
	}
	return name, true
}

// closureHasInvariantParams reports whether any closure parameter
// carries an invariant-bearing type — an enum discriminant or an
// invariant struct, directly or through one nullable layer (story 54,
// RFC-007 §6.9.7).
func (l *lowerer) closureHasInvariantParams(cl *parser.Closure) bool {
	for _, p := range cl.Params {
		if l.paramTypeInvariant(p.TypeExpr) {
			return true
		}
	}
	return false
}

// paramTypeInvariant walks one type spelling: a nullable wrapper keeps
// the element's invariants (the tagged carrier carries the discriminant);
// pointer forms stay outside the v1 scope — native nil is semantic nil,
// and the pointee-invariant question is a documented boundary.
func (l *lowerer) paramTypeInvariant(t *parser.TypeExpr) bool {
	if t == nil {
		return false
	}
	if t.Nullable {
		return l.paramTypeInvariant(t.Elem)
	}
	if t.Kind == parser.NamedType {
		if _, enum := l.enums[t.Name]; enum {
			return true
		}
		if l.structs[t.Name] != nil && l.typeHasInv(t.Name) {
			return true
		}
	}
	return false
}

// wrapCallback renders the validating wrapper literal for a callback
// with invariant-bearing parameters (story 55, RFC-008 §6.9.10): the
// §6.8 boundary checks run before the body literal is invoked, and the
// wrapper stays at the argument position so captures remain lexical.
// Result-carrying and seen-map (pointer-cycle) callbacks stay rejected —
// the result-carrying and seen-map wrapper slices are separate work.
func (l *lowerer) wrapCallback(value parser.Value) (string, error) {
	cl := value.Closure
	if cl.HasResult || len(cl.ResultList) > 0 {
		return "", fmt.Errorf("experimental lowering: result-carrying callback requires the result-carrying wrapper slice")
	}
	stmt := parser.Statement{Kind: parser.Function, Names: []string{"__anuy_cb"}, Closure: cl}
	checks, needsSeen, err := l.boundaryChecks(&stmt)
	if err != nil {
		return "", err
	}
	if needsSeen {
		return "", fmt.Errorf("experimental lowering: callback parameter requires the seen-map wrapper slice")
	}
	if len(checks) == 0 {
		return l.value(value)
	}
	var params strings.Builder
	names := make([]string, 0, len(cl.Params))
	for i, p := range cl.Params {
		if i > 0 {
			params.WriteString(", ")
		}
		typeText := p.Type
		if p.TypeExpr != nil {
			text, err := l.goType(p.TypeExpr)
			if err != nil {
				return "", err
			}
			typeText = text
		}
		params.WriteString(p.Name + " " + typeText)
		names = append(names, p.Name)
	}
	inner, err := l.value(value)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("func(" + params.String() + ") {\n")
	for _, check := range checks {
		b.WriteString(check)
	}
	l.boundaryUsed = true
	b.WriteString("\t" + inner + "(" + strings.Join(names, ", ") + ")\n")
	b.WriteString("}")
	return b.String(), nil
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
		if value.Closure != nil && l.closureHasInvariantParams(value.Closure) {
			// Story 55 (RFC-008 §6.9.10): a callback crossing to Go is a
			// foreign entry (§6.9.7 RFC-007) - the literal lowers to a
			// validating wrapper at the argument position, so captures
			// remain lexical.
			wrapped, werr := l.wrapCallback(value)
			if werr != nil {
				return werr
			}
			args = append(args, wrapped)
			continue
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
	// Story 42 (RFC-009 §6.8.14): a direct call to a wrapped function
	// targets the native entry - the §6.8.5 bypass; the wrapper serves
	// external Go callers only.
	if len(call.Segments) == 0 && l.wrappedFns[call.Receiver] {
		expr = "__anuy_" + expr
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

// funcSignature renders the declaration head of one hoisted top-level
// function or method: `func [recv] name(params) results` (story 16; the
// story 42 wrapper split reuses it for the public and internal symbols).
// forceReceiverName names an unnamed receiver - the wrapper body must
// reference its receiver, while the native entry keeps the source
// spelling (story 45).
func (l *lowerer) funcSignature(statement *parser.Statement, name string, forceReceiverName bool) (string, error) {
	cl := statement.Closure
	var b strings.Builder
	b.WriteString("func ")
	if statement.Method != "" {
		// Story 45 (ADR-0011, RFC-004 §6.1.2): the source receiver lowers
		// verbatim - name when bound, the full type spelling through the
		// representation contract (`*T` stays native, `T?` is the carrier).
		receiverType, err := l.goType(statement.ReceiverType)
		if err != nil {
			return "", err
		}
		recvName := statement.ReceiverName
		if recvName == "" && forceReceiverName {
			recvName = methodReceiver
		}
		if recvName != "" {
			b.WriteString("(" + recvName + " " + receiverType + ") ")
		} else {
			b.WriteString("(" + receiverType + ") ")
		}
	}
	b.WriteString(name + "(")
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
	b.WriteString(")")
	if len(cl.ResultList) > 0 {
		// Story 35 (RFC-005 §6.2.3, RFC-009 §6.6.2): the strict fallible
		// Go ABI - one Go result per declared position, trailing `error`.
		parts := make([]string, 0, len(cl.ResultList))
		for _, t := range cl.ResultList {
			text, err := l.goType(t)
			if err != nil {
				return "", err
			}
			parts = append(parts, text)
		}
		b.WriteString(" (" + strings.Join(parts, ", ") + ")")
	} else if statement.HasResult && cl.ResultTypeExpr != nil {
		text, err := l.goType(cl.ResultTypeExpr)
		if err != nil {
			return "", err
		}
		b.WriteString(" " + text)
	}
	return b.String(), nil
}

// boundaryChecks renders the foreign-entry checks of a declaration
// (story 42, RFC-009 §6.8.2, §6.8.5–6.8.7, §6.8.11–6.8.13; story 43 adds
// aggregates and containers, §6.8.8–6.8.9, §6.8.15–6.8.17): one-level
// invariants from story 42 plus per-type validator delegation. The
// second result reports that the wrapper must own a seen map - any
// parameter shape reaching a cycle-capable aggregate (§6.8.17). The
// `unsafe func` form gets no exemption (§6.8.13).
func (l *lowerer) boundaryChecks(statement *parser.Statement) ([]string, bool, error) {
	var checks []string
	if statement.Method != "" && statement.MethodPointer {
		// §6.8.13: exported method receivers are inside the boundary.
		// The source receiver name carries into the wrapper; the
		// synthetic name only labels the wrapper's own parameter when
		// the source receiver is unnamed/blank (story 45).
		recv := statement.ReceiverName
		if recv == "" {
			recv = methodReceiver
		}
		checks = append(checks, fmt.Sprintf("\tif %s == nil {\n\t\tanuyabi.Require(%q, %q)\n\t}\n", recv, recv, "nil *"+statement.Method))
	}
	needsSeen := false
	for _, p := range statement.Closure.Params {
		if l.shapeNeedsSeen(p.TypeExpr) {
			needsSeen = true
			break
		}
	}
	for _, p := range statement.Closure.Params {
		lines, err := l.shapeChecks(p.Name, p.Name, p.TypeExpr, needsSeen)
		if err != nil {
			return nil, false, err
		}
		checks = append(checks, lines...)
	}
	// The boundaryUsed flag moves to the wrapper-emission site (story 45):
	// discarded checks of non-exported declarations must not force the
	// import.
	return checks, needsSeen, nil
}

// typeHasInv reports the transitive invariant predicate of a declared
// struct (story 43, RFC-009 §6.8.15): true when any field carries a
// one-level invariant or reaches an invariant-bearing aggregate. The
// in-progress mark cuts the fixpoint - a type reachable only through
// itself contributes nothing.
func (l *lowerer) typeHasInv(name string) bool {
	if v, ok := l.hasInvMemo[name]; ok {
		return v
	}
	if l.hasInvWip[name] || l.structs[name] == nil {
		return false
	}
	l.hasInvWip[name] = true
	res := false
	for _, f := range l.structs[name] {
		if l.shapeHasInvariant(f.TypeExpr) {
			res = true
			break
		}
	}
	delete(l.hasInvWip, name)
	l.hasInvMemo[name] = res
	return res
}

// shapeHasInvariant reports whether one field shape contributes an
// entry invariant (RFC-009 §6.8.8, §6.8.15).
func (l *lowerer) shapeHasInvariant(t *parser.TypeExpr) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case parser.NamedType:
		if _, enum := l.enums[t.Name]; enum {
			return true
		}
		if t.Name == "error" || l.interfaces[t.Name] {
			return !t.Nullable
		}
		return l.typeHasInv(t.Name)
	case parser.PointerType:
		if !t.Nullable {
			return true // the nil itself is the invariant
		}
		return l.refHasInvariant(t.Elem)
	case parser.MapType:
		if !t.Nullable {
			return true
		}
		return l.refHasInvariant(t.Key) || l.refHasInvariant(t.Value)
	case parser.SliceType:
		return l.refHasInvariant(t.Elem)
	}
	return false
}

// refHasInvariant reports whether a referenced element shape carries
// invariants: pointer elements nil-check, enum elements range-check,
// aggregate elements validate.
func (l *lowerer) refHasInvariant(t *parser.TypeExpr) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case parser.NamedType:
		if _, enum := l.enums[t.Name]; enum {
			return true
		}
		if t.Name == "error" || l.interfaces[t.Name] {
			return false // dynamic values stay outside the slice
		}
		return l.typeHasInv(t.Name)
	case parser.PointerType:
		return true
	case parser.SliceType:
		return l.refHasInvariant(t.Elem)
	case parser.MapType:
		return l.refHasInvariant(t.Key) || l.refHasInvariant(t.Value)
	}
	return false
}

// namedTargets collects the declared struct names nested anywhere in the
// shape - the all-edges graph of aggregate traversal.
func (l *lowerer) namedTargets(t *parser.TypeExpr, out map[string]bool) {
	if t == nil {
		return
	}
	switch t.Kind {
	case parser.NamedType:
		if l.structs[t.Name] != nil {
			out[t.Name] = true
		}
	case parser.PointerType, parser.SliceType:
		l.namedTargets(t.Elem, out)
	case parser.MapType:
		l.namedTargets(t.Key, out)
		l.namedTargets(t.Value, out)
	}
}

// typeCycleCapable reports whether the struct closure of name can reach
// itself again. A closed traversal path over value-only edges is
// unconstructible in Go (the type would not compile), so any legal cycle
// in the closure surfaces as self-reach over the all-edges graph.
func (l *lowerer) typeCycleCapable(name string) bool {
	visited := map[string]bool{}
	seed := map[string]bool{}
	for _, f := range l.structs[name] {
		l.namedTargets(f.TypeExpr, seed)
	}
	stack := make([]string, 0, len(seed))
	for t := range seed {
		if t == name {
			return true
		}
		stack = append(stack, t)
	}
	for len(stack) > 0 {
		cur := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[cur] || l.structs[cur] == nil {
			continue
		}
		visited[cur] = true
		next := map[string]bool{}
		for _, f := range l.structs[cur] {
			l.namedTargets(f.TypeExpr, next)
		}
		for t := range next {
			if t == name {
				return true
			}
			if !visited[t] {
				stack = append(stack, t)
			}
		}
	}
	return false
}

// typeSeenNeeded reports whether validating the struct closure of name
// can traverse a pointer cycle - the validator then carries the visited
// set (RFC-009 §6.8.9, §6.8.17). The requirement propagates through
// every edge, value edges included.
func (l *lowerer) typeSeenNeeded(name string) bool {
	if v, ok := l.seenMemo[name]; ok {
		return v
	}
	if l.seenWip[name] || l.structs[name] == nil {
		return false
	}
	l.seenWip[name] = true
	res := l.typeCycleCapable(name)
	if !res {
		targets := map[string]bool{}
		for _, f := range l.structs[name] {
			l.namedTargets(f.TypeExpr, targets)
		}
		for t := range targets {
			if l.typeSeenNeeded(t) {
				res = true
				break
			}
		}
	}
	delete(l.seenWip, name)
	l.seenMemo[name] = res
	return res
}

// shapeNeedsSeen reports whether validating the shape requires the
// shared visited set - the wrapper then owns it (story 43, §6.8.17).
func (l *lowerer) shapeNeedsSeen(t *parser.TypeExpr) bool {
	if t == nil {
		return false
	}
	switch t.Kind {
	case parser.NamedType:
		return l.structs[t.Name] != nil && l.typeHasInv(t.Name) && l.typeSeenNeeded(t.Name)
	case parser.PointerType:
		return t.Elem != nil && t.Elem.Kind == parser.NamedType &&
			l.structs[t.Elem.Name] != nil && l.typeHasInv(t.Elem.Name) && l.typeSeenNeeded(t.Elem.Name)
	case parser.SliceType:
		return l.shapeNeedsSeen(t.Elem)
	case parser.MapType:
		return l.shapeNeedsSeen(t.Key) || l.shapeNeedsSeen(t.Value)
	}
	return false
}

// aggregateCall renders the validator invocation for an aggregate value:
// in place through the seen map for cycle-capable closures, by value for
// provably acyclic ones (§6.8.17).
func (l *lowerer) aggregateCall(name, expr string, hasSeen bool) (string, error) {
	if l.typeSeenNeeded(name) {
		if !hasSeen {
			return "", fmt.Errorf("experimental lowering: boundary for %s requires a seen map", name)
		}
		return fmt.Sprintf("\t__anuy_validate%s(&%s, seen)\n", name, expr), nil
	}
	return fmt.Sprintf("\t__anuy_validate%s(%s)\n", name, expr), nil
}

// shapeChecks renders the boundary checks for one value of type t read
// through expr, under the diagnostic label label (story 42/43, RFC-009
// §6.8.5–6.8.9, §6.8.11–6.8.17): parameters, validator fields and
// container elements share one engine. hasSeen reports that the
// enclosing scope owns the shared visited set.
func (l *lowerer) shapeChecks(label, expr string, t *parser.TypeExpr, hasSeen bool) ([]string, error) {
	if t == nil {
		return nil, nil
	}
	switch t.Kind {
	case parser.NamedType:
		if variants, ok := l.enums[t.Name]; ok {
			if !t.Nullable {
				return []string{fmt.Sprintf("\tanuyabi.RequireEnum(%q, uint32(%s), %d)\n", label, expr, variants)}, nil
			}
			return []string{fmt.Sprintf("\tif %s.Present {\n\t\tanuyabi.RequireEnum(%q, uint32(%s.Value), %d)\n\t}\n", expr, label, expr, variants)}, nil
		}
		if t.Name == "error" || l.interfaces[t.Name] {
			if t.Nullable {
				return nil, nil // native-nil class: nil is semantic nil (§6.2.7)
			}
			return []string{fmt.Sprintf("\tanuyabi.RequireNonNilInterface(%q, %s)\n", label, expr)}, nil
		}
		if l.structs[t.Name] == nil || !l.typeHasInv(t.Name) {
			return nil, nil // primitives and invariant-free aggregates (§6.8.15)
		}
		// the tagged payload lives behind the carrier's Value field
		// (§6.8.7); the absent state ignores it
		callExpr := expr
		if t.Nullable {
			callExpr = expr + ".Value"
		}
		call, err := l.aggregateCall(t.Name, callExpr, hasSeen)
		if err != nil {
			return nil, err
		}
		if !t.Nullable {
			return []string{call}, nil
		}
		return []string{fmt.Sprintf("\tif %s.Present {\n%s\t}\n", expr, indentText(call))}, nil
	case parser.PointerType:
		var lines []string
		if !t.Nullable {
			text, err := l.goType(t)
			if err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("\tif %s == nil {\n\t\tanuyabi.Require(%q, %q)\n\t}\n", expr, label, "nil "+text))
		}
		pointee, err := l.pointeeChecks(label, expr, t, hasSeen)
		if err != nil {
			return nil, err
		}
		return append(lines, pointee...), nil
	case parser.MapType:
		var lines []string
		if !t.Nullable {
			text, err := l.goType(t)
			if err != nil {
				return nil, err
			}
			lines = append(lines, fmt.Sprintf("\tif %s == nil {\n\t\tanuyabi.Require(%q, %q)\n\t}\n", expr, label, "nil "+text))
		}
		if !l.refHasInvariant(t.Key) && !l.refHasInvariant(t.Value) {
			return lines, nil
		}
		// ranging over a nil map is zero iterations - the nullable
		// native-nil class needs no guard (§6.2.4)
		keyInv := l.refHasInvariant(t.Key)
		kvar, evar := "_", l.loopVar()
		if keyInv {
			kvar = l.loopVar()
		}
		keyLines, err := l.shapeChecks(label, kvar, t.Key, hasSeen)
		if err != nil {
			return nil, err
		}
		valLines, err := l.shapeChecks(label, evar, t.Value, hasSeen)
		if err != nil {
			return nil, err
		}
		loop := fmt.Sprintf("\tfor %s, %s := range %s {\n", kvar, evar, expr) + indent(append(keyLines, valLines...)) + "\t}\n"
		return append(lines, loop), nil
	case parser.SliceType:
		if !l.refHasInvariant(t.Elem) {
			return nil, nil
		}
		evar := l.loopVar()
		inner, err := l.shapeChecks(label, evar, t.Elem, hasSeen)
		if err != nil {
			return nil, err
		}
		loop := fmt.Sprintf("\tfor _, %s := range %s {\n", evar, expr) + indent(inner) + "\t}\n"
		if t.Nullable {
			// tagged nullable slice: the absent state ignores the payload
			return []string{fmt.Sprintf("\tif %s.Present {\n%s\t}\n", expr, indentText(loop))}, nil
		}
		return []string{loop}, nil
	}
	return nil, nil
}

// pointeeChecks validates the pointee of a pointer whose nil case is
// handled: cycle-capable targets validate in place through the seen map,
// acyclic ones through the value validator on the dereference.
func (l *lowerer) pointeeChecks(label, expr string, t *parser.TypeExpr, hasSeen bool) ([]string, error) {
	e := t.Elem
	if e == nil {
		return nil, nil
	}
	if e.Kind == parser.NamedType && l.structs[e.Name] != nil && l.typeSeenNeeded(e.Name) {
		if !hasSeen {
			return nil, fmt.Errorf("experimental lowering: boundary for %s requires a seen map", label)
		}
		if t.Nullable {
			return []string{fmt.Sprintf("\tif %s != nil {\n\t\t__anuy_validate%s(%s, seen)\n\t}\n", expr, e.Name, expr)}, nil
		}
		return []string{fmt.Sprintf("\t__anuy_validate%s(%s, seen)\n", e.Name, expr)}, nil
	}
	return l.shapeChecks(label, "*"+expr, e, hasSeen)
}

// loopVar mints a fresh deterministic element variable (§13 rule 76).
func (l *lowerer) loopVar() string {
	v := fmt.Sprintf("__anuy_e%d", l.temps)
	l.temps++
	return v
}

func indent(lines []string) string {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("\t" + line)
	}
	return b.String()
}

// indentText prefixes every non-empty line of an already-rendered block
// with one tab - the inline wrapper of single-block indentations.
func indentText(block string) string {
	lines := strings.Split(block, "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "\t" + line
		}
	}
	return strings.Join(lines, "\n")
}

// planValidators collects the per-type validator set (story 43, RFC-009
// §6.8.16): the closure of aggregate types reachable from exported
// signatures through invariant-bearing paths, emitted deterministically
// (§13 rule 76).
func (l *lowerer) planValidators(funcs []parser.Statement) error {
	needed := map[string]bool{}
	for i := range funcs {
		statement := funcs[i]
		if !token.IsExported(statement.Names[0]) {
			continue
		}
		for _, p := range statement.Closure.Params {
			l.seedShape(p.TypeExpr, needed)
		}
	}
	queue := make([]string, 0, len(needed))
	for name := range needed {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[len(queue)-1]
		queue = queue[:len(queue)-1]
		for _, f := range l.structs[name] {
			if !l.shapeHasInvariant(f.TypeExpr) {
				continue
			}
			targets := map[string]bool{}
			l.namedTargets(f.TypeExpr, targets)
			for t := range targets {
				if l.typeHasInv(t) && !needed[t] {
					needed[t] = true
					queue = append(queue, t)
				}
			}
		}
	}
	names := make([]string, 0, len(needed))
	for name := range needed {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if l.validatorsPlanned[name] {
			continue
		}
		l.validatorsPlanned[name] = true
		src, err := l.validatorSource(name)
		if err != nil {
			return err
		}
		l.validators = append(l.validators, anchored{src: src, anchor: l.typeSpans[name], owner: l.typeFiles[name]})
	}
	return nil
}

// seedShape collects the aggregate validators a parameter shape calls
// directly.
func (l *lowerer) seedShape(t *parser.TypeExpr, needed map[string]bool) {
	if t == nil {
		return
	}
	switch t.Kind {
	case parser.NamedType:
		if l.structs[t.Name] != nil && l.typeHasInv(t.Name) {
			needed[t.Name] = true
		}
	case parser.PointerType, parser.SliceType:
		l.seedShape(t.Elem, needed)
	case parser.MapType:
		l.seedShape(t.Key, needed)
		l.seedShape(t.Value, needed)
	}
}

// validatorSource renders one per-type validator (story 43, RFC-009
// §6.8.16): the pointer form with the visited set for cycle-capable
// closures (§6.8.9/§6.8.17), the value form for provably acyclic ones.
func (l *lowerer) validatorSource(name string) (string, error) {
	fields := l.structs[name]
	if fields == nil {
		return "", fmt.Errorf("experimental lowering: validator for undeclared struct %s", name)
	}
	var b strings.Builder
	seen := l.typeSeenNeeded(name)
	if seen {
		fmt.Fprintf(&b, "func __anuy_validate%s(v *%s, seen map[any]struct{}) {\n\tif v == nil {\n\t\treturn\n\t}\n\tif _, dup := seen[any(v)]; dup {\n\t\treturn\n\t}\n\tseen[any(v)] = struct{}{}\n", name, name)
	} else {
		fmt.Fprintf(&b, "func __anuy_validate%s(v %s) {\n", name, name)
	}
	for _, f := range fields {
		if f.TypeExpr == nil {
			continue
		}
		lines, err := l.shapeChecks(name+"."+f.Name, "v."+f.Name, f.TypeExpr, seen)
		if err != nil {
			return "", err
		}
		for _, line := range lines {
			b.WriteString(line)
		}
	}
	b.WriteString("}\n\n")
	return b.String(), nil
}

// boundaryWrapper renders the public Go-facing symbol of a wrapped
// declaration (story 42, RFC-009 §6.8.5): the checks in parameter order,
// then the forwarding call to the internal native entry. needsSeen
// allocates the shared visited set once at the boundary (story 43,
// §6.8.9/§6.8.17).
func (l *lowerer) boundaryWrapper(statement *parser.Statement, name string, checks []string, needsSeen bool) (string, error) {
	cl := statement.Closure
	sig, err := l.funcSignature(statement, name, true)
	if err != nil {
		return "", err
	}
	call := "__anuy_" + name + "("
	for i, p := range cl.Params {
		if i > 0 {
			call += ", "
		}
		call += p.Name
	}
	call += ")"
	if statement.Method != "" {
		recv := statement.ReceiverName
		if recv == "" {
			recv = methodReceiver
		}
		call = recv + "." + call
	}
	if len(cl.ResultList) > 0 || (statement.HasResult && cl.ResultTypeExpr != nil) {
		call = "return " + call
	}
	var b strings.Builder
	b.WriteString(sig + " {\n")
	if needsSeen {
		// the visited set is allocated after the rejecting nil checks but
		// before the first validator call that threads it - an invalid
		// pointer panics without paying the allocation (§6.8.10)
		inserted := false
		ordered := make([]string, 0, len(checks)+1)
		for _, check := range checks {
			if !inserted && strings.Contains(check, ", seen)") {
				ordered = append(ordered, "\tseen := map[any]struct{}{}\n")
				inserted = true
			}
			ordered = append(ordered, check)
		}
		checks = ordered
	}
	for _, check := range checks {
		b.WriteString(check)
	}
	b.WriteString("\t" + call + "\n}\n\n")
	return b.String(), nil
}

// function lowers one hoisted top-level declaration (story 16, RFC-002
// §6.4.11-6.4.12, §6.7-6.8): parameters and the declared result render
// through the representation rules, and the body lowers with the return
// context set to the result's carrier element (empty for native-nil and
// non-null results - raw returns). `//anuy:pure` is a kernel contract and
// a no-op for generated Go. Story 42 (RFC-009 §6.8.1/§6.8.5): an exported
// declaration with one-level invariants lowers to the public validating
// wrapper plus the `__anuy_` internal native entry (§6.5.9); without
// boundary work the wrapper merges into the implementation (§6.5.7) and
// unexported declarations carry no boundary at all (§6.8.13).
func (l *lowerer) function(decls *strings.Builder, statement *parser.Statement) error {
	name := statement.Names[0]
	checks, needsSeen, err := l.boundaryChecks(statement)
	if err != nil {
		return err
	}
	// The test variant (story 67) renders plain bodies: test
	// declarations are not published-API foreign entries.
	if len(checks) == 0 || !token.IsExported(name) || l.testVariant {
		return l.functionBody(decls, statement, name)
	}
	// The wrapper is emitted: its anuyabi.Require checks own the import
	// (story 45 - the flag moved here from boundaryChecks).
	l.boundaryUsed = true
	wrapper, err := l.boundaryWrapper(statement, name, checks, needsSeen)
	if err != nil {
		return err
	}
	decls.WriteString(wrapper)
	return l.functionBody(decls, statement, "__anuy_"+name)
}

func (l *lowerer) functionBody(decls *strings.Builder, statement *parser.Statement, name string) error {
	cl := statement.Closure
	var b strings.Builder
	sig, err := l.funcSignature(statement, name, false)
	if err != nil {
		return err
	}
	b.WriteString(sig)
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
	// Story 53 (RFC-004 §6.3.2): embedded interfaces render as Go
	// interface embedding — the effective method set carries over.
	for _, embed := range decl.Embeds {
		b.WriteString("\t" + embed + "\n")
	}
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
// assumeNonNullOperandText strips the assume_non_nil(...) wrapper from a
// raw value text (story 41): ok=false when the text is not the intrinsic
// shape.
func assumeNonNullOperandText(text string) (string, bool) {
	const prefix = "assume_non_nil("
	if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, ")") {
		return "", false
	}
	return text[len(prefix) : len(text)-1], true
}

func (l *lowerer) value(value parser.Value) (string, error) {
	if value.Closure == nil {
		// Story 41 (RFC-007 §6.6.6): assume_non_nil is a promise, not a
		// runtime check - the value lowers to its operand unchanged.
		if operand, ok := assumeNonNullOperandText(value.Text); ok {
			return operand, nil
		}
		// Story 42 (RFC-009 §6.8.14): value-position calls of wrapped
		// functions retarget to the native entry; the bare-callee prefix
		// guards against navigation text (`a.b(x)` keeps its shape).
		if len(value.Idents) > 0 && l.wrappedFns[value.Idents[0]] && strings.HasPrefix(value.Text, value.Idents[0]+"(") {
			return "__anuy_" + value.Text, nil
		}
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
