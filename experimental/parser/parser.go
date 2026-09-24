// Package parser implements a deliberately restricted experimental grammar.
package parser

import (
	"fmt"
	"strings"

	"github.com/anuy-lang/anuy/internal/semantic"
)

type Span struct{ Start, End int }

// TypeKind identifies the structural kind of a parsed type expression.
type TypeKind uint8

const (
	NamedType TypeKind = iota
	PointerType
	SliceType
	MapType
)

// TypeExpr is the restricted structural type grammar used by the experimental
// parser. Parentheses affect parsing but are not a semantic node.
type TypeExpr struct {
	Kind     TypeKind
	Name     string
	Elem     *TypeExpr
	Key      *TypeExpr
	Value    *TypeExpr
	Nullable bool
	Span     Span
}

// Canonical returns the owner-approved spelling for this restricted type
// grammar. It is intentionally independent of a future general formatter.
func (t *TypeExpr) Canonical() string {
	return formatType(t, false)
}

func formatType(t *TypeExpr, nested bool) string {
	if t == nil {
		return ""
	}
	var text string
	switch t.Kind {
	case NamedType:
		text = t.Name
	case PointerType:
		text = "*" + formatType(t.Elem, true)
	case SliceType:
		text = "[]" + formatType(t.Elem, true)
	case MapType:
		text = "map[" + formatType(t.Key, true) + "]" + formatType(t.Value, true)
	}
	if !t.Nullable {
		return text
	}
	if t.Kind == MapType {
		text = "(" + text + ")?"
	} else {
		text += "?"
	}
	if nested {
		return "(" + text + ")"
	}
	return text
}

// NavigationSegment is one ordinary or safe selector in a parsed navigation
// expression.
type NavigationSegment struct {
	Name string
	Safe bool
	Call bool
	Span Span
}

// NavigationExpr preserves a receiver and selector order for the restricted
// parser slice.
type NavigationExpr struct {
	Receiver string
	// ReceiverSpan covers the receiver identifier only (story 50,
	// RFC-011 §6.2.18 precision ladder): the read-blame target.
	ReceiverSpan Span
	Segments     []NavigationSegment
}

// StructField is one direct field of a struct declaration (RFC-014 §6.2):
// an ordered name plus a restricted structural type. Embedded marks the
// §6.9 `embed T` / `embed *T` form - the name is derived from the type
// and the field is a real direct storage field.
type StructField struct {
	Name     string
	TypeExpr *TypeExpr
	Span     Span
	Embedded bool
}

// StructDecl is a `type N struct { … }` declaration (RFC-014 §6.2).
type StructDecl struct {
	Name   string
	Fields []StructField
	Span   Span
}

// EnumVariant is one variant of an enum declaration (story 30, RFC-006
// §6.1): a bare name in the declaration order.
type EnumVariant struct {
	Name string
	Span Span
}

// EnumDecl is a `type N enum { … }` declaration (story 30, RFC-006 §6.1):
// a closed set of bare variant names.
type EnumDecl struct {
	Name     string
	Variants []EnumVariant
	Span     Span
}

// InterfaceMethod is one signature line of an interface declaration (story
// 39, RFC-004 §6.1.1): `name(params) [result]` - no receiver, no body.
type InterfaceMethod struct {
	Name   string
	Params []Param
	// HasResult/ResultNullable/ResultTypeExpr mirror the optional result
	// type (single result in this slice).
	HasResult      bool
	ResultNullable bool
	ResultTypeExpr *TypeExpr
	Span           Span
}

// InterfaceDecl is an `interface Name { … }` declaration (story 39,
// RFC-004 §6.1.1): a nominal interface with ordered method signatures.
type InterfaceDecl struct {
	Name    string
	Methods []InterfaceMethod
	Span    Span
}

// ImplDecl is the conformance statement `impl Iface for [*]Type` (story
// 39, RFC-004 §6.1.3): no body - the interface methods are ordinary
// methods of the target type.
type ImplDecl struct {
	Interface string
	Type      string // the target type as written, without the leading "*"
	Pointer   bool
	Span      Span
}

// SwitchArm is one `case Enum.Variant:` arm of a switch (story 31/32,
// RFC-006 §6.3–6.4): the pattern names an enum variant; statement arms
// carry a Body, value arms a single Value.
type SwitchArm struct {
	Receiver string
	Variant  string
	Span     Span
	Body     []Statement
	Value    *Value
	// NilArm marks the `case nil:` arm of a nullable-enum switch (story
	// 33, RFC-006 §6.5).
	NilArm bool
}

// SwitchDecl is a `switch <binding> { … }` statement or RHS (story 31/32,
// RFC-006 §6.3–6.4): exhaustive arms over an enum-typed binding. Produces
// marks the value form - arms carry one value line each.
type SwitchDecl struct {
	Scrutinee string
	Produces  bool
	Arms      []SwitchArm
	Span      Span
}

// KeyedLiteral marks a keyed construction value `N{field: expression, …}`
// (RFC-014 §6.3): the raw Text lowers verbatim; Name carries the type the
// fields belong to, Fields the ordered key names and Span the literal's
// source extent (the completeness check, story 22).
type KeyedLiteral struct {
	Name   string
	Fields []string
	Span   Span
}

type Kind uint8

const (
	Var Kind = iota
	Assign
	If
	Read
	Loop
	Break
	Continue
	Block
	Call
	Function
	Return
	TypeDecl
	Switch
	Try
	Interface
	Impl
	UnsafeBlock
)

// ErrorCategory classifies parse-level rejects. Categories are experimental
// and do not fix final diagnostic wording.
type ErrorCategory string

const (
	UnsupportedSyntax         ErrorCategory = "UnsupportedSyntax"
	ShortDeclaration          ErrorCategory = "ShortDeclaration"
	BareDeclaration           ErrorCategory = "BareDeclaration"
	TypedMultipleDeclaration  ErrorCategory = "TypedMultipleDeclaration"
	DuplicateAssignmentTarget ErrorCategory = "DuplicateAssignmentTarget"
	ArityMismatch             ErrorCategory = "ArityMismatch"
	BlankIdentifierRead       ErrorCategory = "BlankIdentifierRead"
)

// Error reports a parse-level reject with its registry code, severity and
// byte offset (CONTRACTS §1.6: codes come from the same registry as the
// semantic diagnostics).
type Error struct {
	Category ErrorCategory
	Code     semantic.Code
	Severity semantic.Severity
	Offset   int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("experimental parser: %s at %d", e.Message, e.Offset)
}

// newError stamps a parse reject with the registry code and severity of
// its category (CONTRACTS §1.6); an unregistered category is a
// programming error, never a codeless diagnostic.
func newError(category ErrorCategory, offset int, message string) *Error {
	desc, ok := semantic.DescriptorFor(semantic.DiagnosticCategory(category))
	if !ok {
		panic("parser: unregistered error category " + category)
	}
	return &Error{Category: category, Code: desc.Code(), Severity: desc.Severity(), Offset: offset, Message: message}
}

// Value is one right-hand side expression of a declaration or assignment.
type Value struct {
	// Text is the raw source text of a non-closure expression.
	Text string
	// Idents lists the binding identifiers referenced by a non-closure
	// expression in source order.
	Idents []string
	// Navigation is non-nil when the value is a recognized selector chain.
	Navigation *NavigationExpr
	// Keyed is non-nil for a keyed construction value `N{field: expression,
	// …}` (story 21, RFC-014 §6.3): the raw Text lowers verbatim and the
	// Idents count only the field-value reads (keys and the type name are
	// not reads).
	Keyed *KeyedLiteral
	// Switch is non-nil for a value-producing switch RHS (story 32,
	// RFC-006 §6.4.2): the class joins the arm values (§6.4.3) and the
	// lowering requires a declared result type (§6.4.4).
	Switch *SwitchDecl
	// Closure is non-nil for closure literals.
	Closure *Closure
	// Span covers the whole value construct (story 50, RFC-011 §6.2.18
	// precision ladder): read diagnostics blame the exact expression,
	// not the enclosing statement line.
	Span Span
}

// Closure is a `func(Params) { Body }` literal.
type Closure struct {
	Params []Param
	Body   []Statement
	Span   Span
	// HasResult/ResultNullable mirror the declaration-form result type
	// (story 08); closures-as-values stay result-less.
	HasResult      bool
	ResultNullable bool
	// ResultTypeExpr is the structural result type (story 16): lowering
	// spells the carrier from it. Nil without a declared result.
	ResultTypeExpr *TypeExpr
	// ResultList is the parenthesized result list `(T1, …, Tn, error?)`
	// (story 35, RFC-005 §6.2.3): non-empty only when the declaration
	// spells one. HasResult/ResultNullable/ResultTypeExpr mirror the
	// trailing element, so single-result consumers keep working; the list
	// gate (trailing `error?`) is parse-level, fallibility stays a kernel
	// verdict. Result-less and single-type declarations leave it empty.
	ResultList []*TypeExpr
}

// Param is a closure parameter. Type preserves source spelling; TypeExpr is
// its restricted structural representation.
type Param struct {
	Name     string
	Type     string
	TypeExpr *TypeExpr
}

type Statement struct {
	Kind  Kind
	Names []string
	// Type is the declared type as raw source text, or "" for inferred
	// declarations. TypeExpr is non-nil for a typed declaration.
	Type     string
	TypeExpr *TypeExpr
	// Values holds right-hand side expressions.
	Values []Value
	// Cond is the if/loop condition as raw source text; CondIdents lists the
	// binding identifiers it references. CondNavigation is non-nil for a
	// recognized selector chain.
	Cond           string
	CondIdents     []string
	CondNavigation *NavigationExpr
	// Body and Else hold the branch statements of an if statement.
	Body []Statement
	Else []Statement
	Span Span
	// Closure is non-nil for a FunctionDecl statement (story 07): the
	// declared function's parameters and body, parsed with the closure
	// grammar. Names[0] is the declared name. Pure records an `//anuy:pure`
	// annotation on the line before the declaration (story 07 effects
	// proposal): the author's trusted contract that the function performs no
	// observable writes.
	Closure *Closure
	Pure    bool
	// UnsafeFunc marks the `unsafe func` declaration form (story 41,
	// RFC-007 §6.7.1): calling the function requires an unsafe context;
	// the body is NOT an implicit unsafe block (§6.7.3).
	UnsafeFunc bool
	// Method is non-empty for a method declaration (story 08, owner
	// decision 2026-09-17, task-1-8-1-2): the receiver type name as written
	// (`User` in `func (u User) age() int`). The declared name is Names[0];
	// methods resolve through the per-type method sets (story 39).
	Method string
	// MethodPointer records the pointer-receiver spelling `func (u *T) m()`
	// (RFC-004 §6.2.1): the receiver kind feeds the method set
	// (impl for T requires value receivers, impl for *T both forms).
	MethodPointer bool
	// ReceiverName is the source receiver binding of a method declaration
	// (story 45, ADR-0011, RFC-004 §6.1.7); empty for the unnamed `(*T)`
	// and blank `(_ *T)` Go forms - the body gets no binding. Receivers
	// carry no scope name outside their own body.
	ReceiverName string
	// ReceiverType is the parsed receiver type (`T`, `*T`, `T?`, `*T?`);
	// non-nil for every method declaration.
	ReceiverType *TypeExpr
	// HasResult records a declared result type (`func f() User`, RFC-002
	// §40 spelling): `return expr` is valid only inside such a declaration.
	HasResult bool
	// ResultNullable records a declared nullable result type (`T?`).
	ResultNullable bool
	// Call is non-nil for a call statement (story 05 variant A): Receiver is
	// the called binding or navigation root, Segments the ordinary member
	// chain whose last segment carries the call. The call value is discarded;
	// the method name is not a binding read. Since story 07 the call carries
	// comma-separated arguments in Values (story 07 grammar, owner decision
	// 2026-09-17); a closure argument may span lines and close its
	// parenthesis on the block closer line.
	Call *NavigationExpr
	// Struct is non-nil for a `type N struct { … }` declaration (story 21,
	// RFC-014 §6.2): Names[0] carries the type name; the declaration hoists
	// to a package-level Go type.
	Struct *StructDecl
	// Enum is non-nil for a `type N enum { … }` declaration (story 30,
	// RFC-006 §6.1): the declaration hoists to a package-level Go type
	// with discriminant constants.
	Enum *EnumDecl
	// Interface is non-nil for an `interface Name { … }` declaration
	// (story 39, RFC-004 §6.1.1): ordered method signatures, no receivers.
	Interface *InterfaceDecl
	// Impl is non-nil for the conformance statement `impl Iface for
	// [*]Type` (story 39, RFC-004 §6.1.3); erased from generated Go.
	Impl *ImplDecl
	// Switch is non-nil for a `switch <binding> { … }` statement (story
	// 31, RFC-006 §6.3): exhaustive arms over an enum-typed binding.
	Switch *SwitchDecl
	// ErrorReturn marks the failure-return form `return error <expr>`
	// (story 34, RFC-005 §6.4.2): the value is the propagated error.
	ErrorReturn bool
	// TryCall is non-nil for a value-try declaration `var v = try F(x)`
	// (story 35, RFC-005 §6.5.1–6.5.2): Names receive the success results
	// on the success path; the trailing `error?` propagates from the
	// enclosing fallible function (kernel: ANUY6001, D-2). Values holds
	// the call arguments.
	TryCall *NavigationExpr
	// Discard marks the explicit-ignore form `discard <call>` (story 37,
	// RFC-005 §6.6.2): the call's effects apply, its error result is
	// intentionally ignored - no D-1 (kernel).
	Discard bool
	// Target is non-nil for a field-mutation assignment `u.f = expr`
	// (story 22, RFC-014 §6.7): an ordinary single-field navigation path;
	// Names stays empty.
	Target *NavigationExpr
}

// A Loop statement reuses fields by loop form (spec 1-3-1-1): condition form
// sets Cond/CondIdents/CondNavigation; iteration form `for item in xs`
// sets Names to the binding and Values to the collection expression;
// infinite form leaves both empty. Body holds the loop body.

type Program struct{ Statements []Statement }

// Parse accepts typed declarations, single and multiple assignment, closure
// literals in single-value initializers, if/else statements with blocks, the
// three loop forms `for [Expression] Block` / `for identifier in Expression
// Block` with bare `break`/`continue`, and bare identifier reads. Statements
// are line oriented; blocks open with `{` at the end of a header line and
// close with a `}` line. Expressions and conditions retain raw source text;
// recognized navigation chains additionally receive restricted structural
// validation and metadata.
func Parse(source string) (Program, error) {
	var lines []sourceLine
	offset := 0
	for _, raw := range strings.SplitAfter(source, "\n") {
		lines = append(lines, sourceLine{raw: raw, text: strings.TrimSpace(raw), offset: offset})
		offset += len(raw)
	}
	lp := &lineParser{lines: lines}
	statements, err := lp.parseStatements()
	if err != nil {
		return Program{}, err
	}
	return Program{Statements: statements}, nil
}

type sourceLine struct {
	raw    string
	text   string
	offset int
}

type lineParser struct {
	lines []sourceLine
	pos   int
	// loopDepth tracks syntactic loop nesting for `break`/`continue`.
	// It resets to zero inside closure literals: a jump never crosses a
	// function boundary.
	loopDepth int
	// funcStack tracks the declared results of the enclosing
	// function/method bodies (story 08): `return expr` is valid only when
	// the top frame carries a result. Closure-literal frames push a
	// result-less frame, so a `return expr` binds to the nearest
	// declaration, never to an outer one. Since story 35 the frame also
	// carries the result count (multi-value return arity, RFC-005 §6.4.1)
	// and the parse-level fallibility spelling (trailing `error?`).
	funcStack []returnFrame
}

// returnFrame is one funcStack entry: the enclosing declaration's result
// shape as far as the parse layer knows it.
type returnFrame struct {
	hasResult bool
	results   int
	fallible  bool
}

func (lp *lineParser) parseStatements() ([]Statement, error) {
	var out []Statement
	for lp.pos < len(lp.lines) {
		line := lp.lines[lp.pos]
		if line.text == "" {
			lp.pos++
			continue
		}
		if strings.HasPrefix(line.text, "//") {
			lp.pos++
			continue
		}
		if line.text == "}" {
			return nil, newError(UnsupportedSyntax, line.offset, "unexpected }")
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, err
		}
		out = append(out, statement)
	}
	return out, nil
}

// parseBlock parses statements until the closing } line, which it consumes.
// A `} else {` line also closes the block; the header is left for the if
// statement that owns this block.
func (lp *lineParser) parseBlock() ([]Statement, error) {
	var out []Statement
	for {
		if lp.pos >= len(lp.lines) {
			return nil, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		line := lp.lines[lp.pos]
		if line.text == "" {
			lp.pos++
			continue
		}
		if line.text == "}" {
			lp.pos++
			return out, nil
		}
		if lp.isElseHeader(line) || lp.isElseIfHeader(line) {
			return out, nil
		}
		if strings.HasPrefix(line.text, "//") {
			lp.pos++
			continue
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, err
		}
		out = append(out, statement)
	}
}

// parseBlockWithSuffix parses statements like parseBlock, but the closing
// line may carry tokens after `}`: a call argument closes its parenthesis on
// the block closer line (story 07, RFC-003 §41 — `})`). It returns the block
// statements, the closer line's trailing tokens, and the closer line itself.
func (lp *lineParser) parseBlockWithSuffix() ([]Statement, []token, sourceLine, error) {
	var out []Statement
	for {
		if lp.pos >= len(lp.lines) {
			return nil, nil, sourceLine{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		line := lp.lines[lp.pos]
		if line.text == "" {
			lp.pos++
			continue
		}
		if strings.HasPrefix(line.text, "//") {
			lp.pos++
			continue
		}
		if line.text == "}" {
			lp.pos++
			return out, nil, line, nil
		}
		if strings.HasPrefix(line.text, "}") && !lp.isElseHeader(line) && !lp.isElseIfHeader(line) {
			lp.pos++
			tokens, terr := tokenize(line.raw, line.offset)
			if terr != nil {
				return nil, nil, sourceLine{}, terr
			}
			return out, tokens[1:], line, nil
		}
		if lp.isElseHeader(line) || lp.isElseIfHeader(line) {
			return out, nil, line, nil
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, nil, sourceLine{}, err
		}
		out = append(out, statement)
	}
}

// parseStatement dispatches one line. Statement parsers advance lp.pos past
// every line they consume, including multi-line if and closure statements.
func (lp *lineParser) parseStatement(line sourceLine) (Statement, error) {
	tokens, terr := tokenize(line.raw, line.offset)
	if terr != nil {
		return Statement{}, terr
	}
	switch {
	case tokens[0].kind == tokenIdent && tokens[0].text == "var":
		return lp.parseVar(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "if":
		return lp.parseIf(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "for":
		return lp.parseLoop(tokens, line)
	case tokens[0].kind == tokenIdent && (tokens[0].text == "break" || tokens[0].text == "continue"):
		return lp.parseJump(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "unsafe" && len(tokens) == 2 && isPunct(tokens[1], "{"):
		// Story 41 (RFC-007 §6.6.1): the lexical unsafe context block.
		start := line.offset
		lp.pos++
		body, err := lp.parseBlock()
		if err != nil {
			return Statement{}, err
		}
		return Statement{Kind: UnsafeBlock, Body: body, Span: Span{Start: start, End: line.offset + len(line.text)}}, nil
	case tokens[0].kind == tokenIdent && tokens[0].text == "unsafe" && len(tokens) >= 3 && tokens[1].kind == tokenIdent && tokens[1].text == "func":
		// Story 41 (RFC-007 §6.7.1): `unsafe func` - caller-side
		// preconditions; the declaration itself parses like a function.
		statement, err := lp.parseFunctionDecl(tokens[1:], line)
		if err != nil {
			return Statement{}, err
		}
		statement.UnsafeFunc = true
		return statement, nil
	case tokens[0].kind == tokenIdent && tokens[0].text == "unsafe":
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "unsafe requires a block or func declaration")
	case tokens[0].kind == tokenIdent && tokens[0].text == "func" && len(tokens) >= 2 && (tokens[1].kind == tokenIdent || isPunct(tokens[1], "*") || isPunct(tokens[1], "(")):
		// `func name(params) {` — the story 07 declaration form — and the
		// Go receiver method form `func (r [*]T) name(...)` (story 45,
		// RFC-004 §6.1.2); the dotted transitional form is rejected inside
		// with a receiver-form hint.
		return lp.parseFunctionDecl(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "switch":
		// Story 31 (RFC-006 §6.3): the exhaustive enum switch.
		return lp.parseSwitch(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "try":
		// Story 34 (RFC-005 §6.5.3): error-only `try <call>` - the call
		// plus immediate propagation; fallibility is a kernel check
		// (ANUY6001).
		if len(tokens) == 1 {
			return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "try requires a call")
		}
		statement, err := lp.parseCallStatement(tokens[1:], line)
		if err != nil {
			return Statement{}, err
		}
		statement.Kind = Try
		return statement, nil
	case tokens[0].kind == tokenIdent && tokens[0].text == "discard" && !(len(tokens) >= 2 && isPunct(tokens[1], "(")):
		// Story 37 (RFC-005 §6.6.2): the explicit-ignore form
		// `discard <call>` - the call plus the programmer's intent to drop
		// every result. `discard(` stays a call of a binding named
		// discard (the `try` contextual-keyword precedent), so the case
		// requires the next token to open a call chain.
		if len(tokens) < 2 || !isCallStatementStart(tokens[1:]) {
			return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "discard requires a call")
		}
		statement, err := lp.parseCallStatement(tokens[1:], line)
		if err != nil {
			return Statement{}, err
		}
		statement.Discard = true
		return statement, nil
	case tokens[0].kind == tokenIdent && tokens[0].text == "return":
		// `return` is a contextual keyword in statement position (the `in`
		// precedent, story 03).
		return lp.parseReturn(tokens, line)
	case tokens[0].kind == tokenPunct && tokens[0].text == "{":
		if len(tokens) != 1 {
			return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "unexpected token after {")
		}
		start := line.offset
		lp.pos++
		body, err := lp.parseBlock()
		if err != nil {
			return Statement{}, err
		}
		return Statement{Kind: Block, Body: body, Span: Span{Start: start, End: line.offset + len(line.text)}}, nil
	case tokens[0].kind == tokenBlank && len(tokens) == 1:
		// A bare `_` is a read position: the blank identifier holds no value.
		return Statement{}, newError(BlankIdentifierRead, tokens[0].start, "_ does not hold a value")
	case tokens[0].kind == tokenBlank && len(tokens) >= 3 && isPunct(tokens[1], "("):
		// `_()` — the blank identifier is not callable (GB-3: `_` holds no
		// value, so it cannot be read as a callee either).
		return Statement{}, newError(BlankIdentifierRead, tokens[0].start, "_ does not hold a value")
	case tokens[0].kind == tokenIdent && len(tokens) >= 2 && isPunct(tokens[1], ".") && hasTopLevelAssign(tokens):
		// Story 22: `u.f = expr` — field mutation, before the navigation-
		// call path intercepts the dot.
		return lp.parseFieldAssignment(tokens, line)
	case isCallStatementStart(tokens):
		return lp.parseCallStatement(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "type":
		return lp.parseTypeDecl(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "interface":
		// Story 39 (RFC-004 §6.1.1): the nominal interface declaration.
		return lp.parseInterfaceDecl(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "impl":
		// Story 39 (RFC-004 §6.1.3): the explicit conformance statement.
		return lp.parseImplDecl(tokens, line)
	case tokens[0].kind == tokenIdent && len(tokens) >= 2 && isPunct(tokens[1], ".") && hasTopLevelAssign(tokens):
		return lp.parseFieldAssignment(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "else":
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "unexpected else")
	case len(tokens) == 1 && tokens[0].kind == tokenIdent:
		lp.pos++
		return Statement{
			Kind:  Read,
			Names: []string{tokens[0].text},
			Span:  Span{Start: line.offset, End: line.offset + len(line.text)},
		}, nil
	default:
		return lp.parseAssign(tokens, line)
	}
}

// isCallStatementStart reports a line beginning with `identifier(` or
// `identifier.` — a call statement candidate (story 05 variant A). Closure
// literals (`func (`) are rejected inside parseCallStatement.
func isCallStatementStart(tokens []token) bool {
	if len(tokens) < 3 || tokens[0].kind != tokenIdent || reservedWords[tokens[0].text] {
		return false
	}
	if isPunct(tokens[1], "(") || isPunct(tokens[1], ".") {
		return true
	}
	if isPunct(tokens[1], "?") {
		// A safe-tail call statement (story 08 Q3-A): `u?.m()`. Lines that
		// carry an assignment keep the assignment errors - the safe-
		// navigation target rejection preserves its offset.
		for _, t := range tokens {
			if isAssign(t) {
				return false
			}
		}
		return true
	}
	return false
}

// parseFunctionDecl parses the story 07 declaration form `func name(params)
// [result] {` or the Go receiver method form `func (r [*]T) name(params)
// [result] {` (story 45, ADR-0011, RFC-004 §6.1.2). The receiver group
// reuses the parameter type grammar; unnamed `(*T)` and blank `(_ *T)`
// receivers bind nothing (RFC-004 §6.1.7). The transitional dotted
// `func [*]T.m` form is rejected with a receiver-form hint.
func (lp *lineParser) parseFunctionDecl(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) >= 3 && isPunct(tokens[1], "(") {
		return lp.parseReceiverMethod(tokens, line)
	}
	if len(tokens) >= 4 && isPunct(tokens[2], ".") {
		return Statement{}, newError(UnsupportedSyntax, tokens[2].start, "method declaration uses the Go receiver form: func (s T) m()")
	}
	if len(tokens) >= 5 && isPunct(tokens[1], "*") && isPunct(tokens[3], ".") {
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "method declaration uses the Go receiver form: func (s *T) m()")
	}
	name := tokens[1]
	if name.text == "_" || isReservedName(name.text) {
		return Statement{}, newError(UnsupportedSyntax, name.start, "invalid function name")
	}
	params := 2
	if params >= len(tokens) || !isPunct(tokens[params], "(") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "function declaration requires a parameter list")
	}
	pure := lp.pos > 0 && lp.lines[lp.pos-1].text == "//anuy:pure"
	// Reuse the closure parser on the token slice without the name: token
	// offsets are absolute, so parameter types keep their source spelling.
	shifted := make([]token, 0, len(tokens)-params+1)
	shifted = append(shifted, tokens[0])
	shifted = append(shifted, tokens[params:]...)
	cl, cerr := lp.parseClosure(shifted, 0, line, true)
	if cerr != nil {
		return Statement{}, cerr
	}
	return Statement{
		Kind:           Function,
		Names:          []string{name.text},
		HasResult:      cl.HasResult,
		ResultNullable: cl.ResultNullable,
		Closure:        &cl,
		Pure:           pure,
		Span:           Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseReceiverMethod parses the receiver group `(r [*]T [?])` of a method
// declaration (story 45, RFC-004 §6.1.2/§6.1.7): the leading identifier is
// the binding name, a reserved word or `_` marks the unnamed/blank forms,
// and the rest is the receiver type. The base of the receiver type names
// the per-type method set (§6.2.1).
func (lp *lineParser) parseReceiverMethod(tokens []token, line sourceLine) (Statement, error) {
	close := -1
	for i := 2; i < len(tokens); i++ {
		if isPunct(tokens[i], ")") {
			close = i
			break
		}
	}
	if close < 0 {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "unterminated receiver list")
	}
	group := tokens[2:close]
	if len(group) == 0 {
		// `func() { }` - an anonymous closure is not a declaration (the
		// pre-story-45 rejection stands).
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "receiver requires a type")
	}
	name := ""
	typeTokens := group
	switch {
	case group[0].kind == tokenBlank:
		// the blank receiver binds nothing
		typeTokens = group[1:]
	case len(group) >= 2 && group[0].kind == tokenIdent && !reservedWords[group[0].text]:
		name = group[0].text
		typeTokens = group[1:]
	}
	if len(typeTokens) == 0 {
		return Statement{}, newError(UnsupportedSyntax, group[0].start, "receiver requires a type")
	}
	receiverType, terr := parseType(typeTokens)
	if terr != nil {
		return Statement{}, terr
	}
	if receiverType.Kind == NamedType && receiverType.Nullable {
		// RFC-004 §6.1.7: the generated Go cannot define methods on the
		// carrier type (RFC-009 §6.7) - a nullable receiver uses the
		// pointer spelling `*T?`.
		return Statement{}, newError(UnsupportedSyntax, receiverType.Span.Start, "nullable value receiver is not a receiver form; use *T?")
	}
	base := receiverType
	if base.Kind == PointerType {
		base = base.Elem
	}
	if base == nil || base.Kind != NamedType || base.Name == "" {
		return Statement{}, newError(UnsupportedSyntax, receiverType.Span.Start, "receiver must be a named type or a pointer to one")
	}
	if close+1 >= len(tokens) || tokens[close+1].kind != tokenIdent || tokens[close+1].text == "_" || isReservedName(tokens[close+1].text) {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "invalid method name")
	}
	m := tokens[close+1]
	pure := lp.pos > 0 && lp.lines[lp.pos-1].text == "//anuy:pure"
	// Reuse the closure parser from the parameter list, dropping the
	// receiver group and the method name: token offsets are absolute, so
	// parameter types keep their source spelling.
	shifted := make([]token, 0, len(tokens)-close-1)
	shifted = append(shifted, tokens[0])
	shifted = append(shifted, tokens[close+2:]...)
	cl, cerr := lp.parseClosure(shifted, 0, line, true)
	if cerr != nil {
		return Statement{}, cerr
	}
	return Statement{
		Kind:           Function,
		Names:          []string{m.text},
		Method:         base.Name,
		MethodPointer:  receiverType.Kind == PointerType,
		ReceiverName:   name,
		ReceiverType:   receiverType,
		HasResult:      cl.HasResult,
		ResultNullable: cl.ResultNullable,
		Closure:        &cl,
		Pure:           pure,
		Span:           Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseTypeDecl parses the `type N struct {` declaration (story 21,
// RFC-014 §6.2): the header line opens the field block, each field line is
// `name type` and the `}` line closes. Zero-field structs are valid;
// duplicate, blank and reserved field names reject. Other type-declaration
// forms (`type N T`, `type N = T`) are a follow-up slice.
func (lp *lineParser) parseTypeDecl(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) < 2 || tokens[1].kind != tokenIdent || tokens[1].text == "_" || reservedWords[tokens[1].text] {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "invalid type name")
	}
	if len(tokens) < 4 || tokens[2].kind != tokenIdent || tokens[2].text == "_" || reservedWords[tokens[2].text] {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "type declaration supports only `type N struct { ... }` and `type N enum { ... }` in this slice")
	}
	if kind := tokens[2].text; kind != "struct" && kind != "enum" {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "type declaration supports only `type N struct { ... }` and `type N enum { ... }` in this slice")
	}
	if !isPunct(tokens[3], "{") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "type declaration supports only `type N struct { ... }` and `type N enum { ... }` in this slice")
	}
	if tokens[2].text == "enum" {
		return lp.parseEnumDecl(tokens, line)
	}
	decl := &StructDecl{Name: tokens[1].text, Span: Span{Start: tokens[0].start}}
	lp.pos++
	seen := map[string]bool{}
	for {
		if lp.pos >= len(lp.lines) {
			return Statement{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		fieldLine := lp.lines[lp.pos]
		if fieldLine.text == "" {
			lp.pos++
			continue
		}
		if fieldLine.text == "}" {
			lp.pos++
			decl.Span.End = fieldLine.offset + len(fieldLine.text)
			return Statement{Kind: TypeDecl, Names: []string{decl.Name}, Struct: decl, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
		}
		if strings.HasPrefix(fieldLine.text, "//") {
			lp.pos++
			continue
		}
		fieldTokens, terr := tokenize(fieldLine.raw, fieldLine.offset)
		if terr != nil {
			return Statement{}, terr
		}
		if fieldTokens[0].kind == tokenIdent && fieldTokens[0].text == "embed" {
			// Story 29 (RFC-014 §6.9): `embed` is a contextual keyword of
			// the struct body - the first token of a field line only; the
			// derived field name is the type name, `*` stripped (§6.10
			// forbids nullable embedding).
			if len(fieldTokens) < 2 {
				return Statement{}, newError(UnsupportedSyntax, fieldTokens[0].start, "embedded field requires a type")
			}
			typeExpr, terr := parseType(fieldTokens[1:])
			if terr != nil {
				return Statement{}, terr
			}
			embeddedName := ""
			if typeExpr != nil {
				switch typeExpr.Kind {
				case NamedType:
					if !typeExpr.Nullable {
						embeddedName = typeExpr.Name
					}
				case PointerType:
					if typeExpr.Elem != nil && typeExpr.Elem.Kind == NamedType && !typeExpr.Elem.Nullable {
						embeddedName = typeExpr.Elem.Name
					}
				}
			}
			if embeddedName == "" {
				return Statement{}, newError(UnsupportedSyntax, fieldTokens[1].start, "embedded field requires a non-null named type")
			}
			if seen[embeddedName] {
				return Statement{}, newError(UnsupportedSyntax, fieldTokens[1].start, fmt.Sprintf("duplicate field %q", embeddedName))
			}
			seen[embeddedName] = true
			decl.Fields = append(decl.Fields, StructField{Name: embeddedName, TypeExpr: typeExpr, Span: Span{Start: fieldTokens[0].start, End: typeExpr.Span.End}, Embedded: true})
			lp.pos++
			continue
		}
		if len(fieldTokens) < 2 || fieldTokens[0].kind != tokenIdent || fieldTokens[0].text == "_" || reservedWords[fieldTokens[0].text] {
			return Statement{}, newError(UnsupportedSyntax, fieldTokens[0].start, "struct field requires a name and a type")
		}
		if seen[fieldTokens[0].text] {
			return Statement{}, newError(UnsupportedSyntax, fieldTokens[0].start, fmt.Sprintf("duplicate field %q", fieldTokens[0].text))
		}
		typeExpr, terr := parseType(fieldTokens[1:])
		if terr != nil {
			return Statement{}, terr
		}
		seen[fieldTokens[0].text] = true
		decl.Fields = append(decl.Fields, StructField{Name: fieldTokens[0].text, TypeExpr: typeExpr, Span: Span{Start: fieldTokens[0].start, End: typeExpr.Span.End}})
		lp.pos++
	}
}

// parseEnumDecl parses the variant body of `type N enum {` (story 30,
// RFC-006 §6.1): bare variant names, one per line; unique (§6.1.4), at
// least one (§6.1.5), no payloads (§6.1.6). Blank lines and `//`
// comments are skipped.
func (lp *lineParser) parseEnumDecl(tokens []token, line sourceLine) (Statement, error) {
	decl := &EnumDecl{Name: tokens[1].text, Span: Span{Start: tokens[0].start}}
	lp.pos++
	seen := map[string]bool{}
	for {
		if lp.pos >= len(lp.lines) {
			return Statement{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		variantLine := lp.lines[lp.pos]
		if variantLine.text == "" || strings.HasPrefix(variantLine.text, "//") {
			lp.pos++
			continue
		}
		if variantLine.text == "}" {
			if len(decl.Variants) == 0 {
				return Statement{}, newError(UnsupportedSyntax, variantLine.offset, "enum requires at least one variant")
			}
			lp.pos++
			decl.Span.End = variantLine.offset + len(variantLine.text)
			return Statement{Kind: TypeDecl, Names: []string{decl.Name}, Enum: decl, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
		}
		variantTokens, terr := tokenize(variantLine.raw, variantLine.offset)
		if terr != nil {
			return Statement{}, terr
		}
		if len(variantTokens) != 1 || variantTokens[0].kind != tokenIdent || variantTokens[0].text == "_" || reservedWords[variantTokens[0].text] {
			return Statement{}, newError(UnsupportedSyntax, variantTokens[0].start, "enum variant must be a bare name")
		}
		if seen[variantTokens[0].text] {
			return Statement{}, newError(UnsupportedSyntax, variantTokens[0].start, fmt.Sprintf("duplicate variant %q", variantTokens[0].text))
		}
		seen[variantTokens[0].text] = true
		decl.Variants = append(decl.Variants, EnumVariant{Name: variantTokens[0].text, Span: Span{Start: variantTokens[0].start, End: variantTokens[0].end}})
		lp.pos++
	}
}

// parseSwitch parses `switch <binding> { case Enum.Variant: … }` (story
// 31, RFC-006 §6.3): exhaustive arms over an enum-typed binding.
// Wildcard/default arms, guards and multi-pattern cases reject (§6.3.6,
// §6.4.5, §6.4.7) - the API evolution guarantee is the feature. `case`
// is a contextual keyword of the switch body.
func (lp *lineParser) parseSwitch(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) != 3 || tokens[1].kind != tokenIdent || tokens[1].text == "_" || reservedWords[tokens[1].text] || !isPunct(tokens[2], "{") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "switch requires `switch <binding> {` in this slice")
	}
	sw := &SwitchDecl{Scrutinee: tokens[1].text, Span: Span{Start: tokens[0].start}}
	lp.pos++
	for {
		if lp.pos >= len(lp.lines) {
			return Statement{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		armLine := lp.lines[lp.pos]
		if armLine.text == "" || strings.HasPrefix(armLine.text, "//") {
			lp.pos++
			continue
		}
		if armLine.text == "}" {
			lp.pos++
			if len(sw.Arms) == 0 {
				return Statement{}, newError(UnsupportedSyntax, armLine.offset, "switch requires at least one case")
			}
			sw.Span.End = armLine.offset + len(armLine.text)
			return Statement{Kind: Switch, Switch: sw, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
		}
		armTokens, terr := tokenize(armLine.raw, armLine.offset)
		if terr != nil {
			return Statement{}, terr
		}
		if len(armTokens) == 3 && armTokens[0].text == "case" && armTokens[1].text == "nil" && isPunct(armTokens[2], ":") {
			// Story 33 (RFC-006 §6.5): `case nil:` arms the nil case of a
			// nullable-enum switch.
			lp.pos++
			body, err := lp.parseSwitchArmBody()
			if err != nil {
				return Statement{}, err
			}
			sw.Arms = append(sw.Arms, SwitchArm{NilArm: true, Span: Span{Start: armTokens[0].start, End: armTokens[2].end}, Body: body})
			continue
		}
		if len(armTokens) != 5 || armTokens[0].text != "case" || armTokens[1].kind != tokenIdent || reservedWords[armTokens[1].text] || !isPunct(armTokens[2], ".") || armTokens[3].kind != tokenIdent || reservedWords[armTokens[3].text] || !isPunct(armTokens[4], ":") {
			return Statement{}, newError(UnsupportedSyntax, armTokens[0].start, "switch arm must be `case Enum.Variant:`")
		}
		lp.pos++
		body, err := lp.parseSwitchArmBody()
		if err != nil {
			return Statement{}, err
		}
		sw.Arms = append(sw.Arms, SwitchArm{
			Receiver: armTokens[1].text,
			Variant:  armTokens[3].text,
			Span:     Span{Start: armTokens[0].start, End: armTokens[4].end},
			Body:     body,
		})
	}
}

// parseSwitchArmBody parses arm-body statements until the next `case`
// line or the switch-closing `}` line, which the caller consumes. Nested
// blocks are consumed whole by parseStatement, so no depth tracking is
// needed here.
func (lp *lineParser) parseSwitchArmBody() ([]Statement, error) {
	var out []Statement
	for {
		if lp.pos >= len(lp.lines) {
			return nil, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		line := lp.lines[lp.pos]
		if line.text == "" || strings.HasPrefix(line.text, "//") {
			lp.pos++
			continue
		}
		if line.text == "}" || strings.HasPrefix(line.text, "case ") {
			return out, nil
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, err
		}
		out = append(out, statement)
	}
}

// parseSwitchValue parses a value-producing switch RHS (story 32,
// RFC-006 §6.4.2): `switch <binding> {` with `case Enum.Variant:` arms
// that produce exactly one value line each. The value class is the
// kernel-side join of the arm values (§6.4.3); the lowering requires the
// declared result type (§6.4.4).
func (lp *lineParser) parseSwitchValue(tokens []token, start int, line sourceLine) (Value, error) {
	if len(tokens)-start != 3 || tokens[start+1].kind != tokenIdent || tokens[start+1].text == "_" || reservedWords[tokens[start+1].text] || !isPunct(tokens[start+2], "{") {
		return Value{}, newError(UnsupportedSyntax, listEnd(tokens), "value switch requires `switch <binding> {` in this slice")
	}
	sw := &SwitchDecl{Scrutinee: tokens[start+1].text, Produces: true, Span: Span{Start: tokens[start].start}}
	lp.pos++
	idents := []string{sw.Scrutinee}
	for {
		if lp.pos >= len(lp.lines) {
			return Value{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		armLine := lp.lines[lp.pos]
		if armLine.text == "" || strings.HasPrefix(armLine.text, "//") {
			lp.pos++
			continue
		}
		if armLine.text == "}" {
			lp.pos++
			if len(sw.Arms) == 0 {
				return Value{}, newError(UnsupportedSyntax, armLine.offset, "switch requires at least one case")
			}
			sw.Span.End = armLine.offset + len(armLine.text)
			return Value{Idents: idents, Switch: sw}, nil
		}
		armTokens, terr := tokenize(armLine.raw, armLine.offset)
		if terr != nil {
			return Value{}, terr
		}
		if len(armTokens) != 5 || armTokens[0].text != "case" || armTokens[1].kind != tokenIdent || reservedWords[armTokens[1].text] || !isPunct(armTokens[2], ".") || armTokens[3].kind != tokenIdent || reservedWords[armTokens[3].text] || !isPunct(armTokens[4], ":") {
			return Value{}, newError(UnsupportedSyntax, armTokens[0].start, "switch arm must be `case Enum.Variant:`")
		}
		lp.pos++
		if lp.pos >= len(lp.lines) {
			return Value{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		valueLine := lp.lines[lp.pos]
		if valueLine.text == "" || valueLine.text == "}" || strings.HasPrefix(valueLine.text, "case ") || strings.HasPrefix(valueLine.text, "switch") {
			return Value{}, newError(UnsupportedSyntax, valueLine.offset, "switch arm must produce a value")
		}
		valueTokens, terr := tokenize(valueLine.raw, valueLine.offset)
		if terr != nil {
			return Value{}, terr
		}
		values, verr := parseValueList(valueTokens, 0, valueLine)
		if verr != nil {
			return Value{}, verr
		}
		if len(values) != 1 {
			return Value{}, newError(UnsupportedSyntax, valueTokens[0].start, "switch arm must produce a single value")
		}
		idents = append(idents, values[0].Idents...)
		lp.pos++
		sw.Arms = append(sw.Arms, SwitchArm{
			Receiver: armTokens[1].text,
			Variant:  armTokens[3].text,
			Span:     Span{Start: armTokens[0].start, End: armTokens[4].end},
			Value:    &values[0],
		})
	}
}

// parseReturn parses the `return` statement. The bare form is an early
// exit of the enclosing function (story 07); `return expr` (story 08) is
// valid only inside a declaration with a declared result type - the
// funcStack frame of the nearest closure/function boundary decides. Since
// story 35 (RFC-005 §6.4.1) a result list takes `return v1, …, nil`: the
// value count must equal the declared result count (ANUY1006).
func (lp *lineParser) parseReturn(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) == 1 {
		lp.pos++
		return Statement{
			Kind: Return,
			Span: Span{Start: line.offset, End: line.offset + len(line.text)},
		}, nil
	}
	if len(tokens) >= 2 && tokens[1].kind == tokenIdent && tokens[1].text == "error" {
		// Story 34 (RFC-005 §6.4.2): the failure-return form
		// `return error <expr>` - `error` is a contextual keyword in
		// return position. Fallibility is a kernel check (ANUY6001). A
		// binding spelled `error` cannot be returned bare (ADR-0009 §6).
		if len(tokens) == 2 {
			return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return error requires an error expression; rename the binding (convention: err) or write return error <expr>")
		}
		if len(lp.funcStack) == 0 {
			return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return takes no value")
		}
		values, verr := parseValueList(tokens, 2, line)
		if verr != nil {
			return Statement{}, verr
		}
		if len(values) != 1 {
			return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return error takes a single value")
		}
		lp.pos++
		return Statement{
			Kind:        Return,
			ErrorReturn: true,
			Values:      values,
			Span:        Span{Start: line.offset, End: line.offset + len(line.text)},
		}, nil
	}
	frame := returnFrame{}
	if len(lp.funcStack) > 0 {
		frame = lp.funcStack[len(lp.funcStack)-1]
	}
	if len(lp.funcStack) == 0 || !frame.hasResult {
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return takes no value")
	}
	values, verr := parseValueList(tokens, 1, line)
	if verr != nil {
		return Statement{}, verr
	}
	if frame.results > 1 {
		// Story 35 (RFC-005 §6.4.1): `return v1, …, nil` - one value per
		// declared result, the trailing one the error slot.
		if len(values) != frame.results {
			return Statement{}, newError(ArityMismatch, tokens[1].start, fmt.Sprintf("return takes %d values, got %d", frame.results, len(values)))
		}
	} else if len(values) != 1 {
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return takes a single value")
	}
	lp.pos++
	return Statement{
		Kind:   Return,
		Values: values,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseCallStatement parses the call statement forms `identifier(args…)`
// and `identifier.segment…(args…)` (story 05 variant A, arguments added in
// story 07): exactly one ordinary call at the end of the chain, value
// discarded. Arguments are comma-separated values of the existing value
// grammar; a closure argument may span lines and close its parenthesis on
// the block closer line (RFC-003 §41, owner decision 2026-09-17).
func (lp *lineParser) parseCallStatement(tokens []token, line sourceLine) (Statement, error) {
	if isClosureStart(tokens, 0) {
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "closure literal is not a statement")
	}
	var segments []NavigationSegment
	callOpen := -1
	if isPunct(tokens[1], "(") {
		callOpen = 1
	} else {
		var scanErr *Error
		segments, callOpen, scanErr = callChainScan(tokens)
		if scanErr != nil {
			return Statement{}, scanErr
		}
		if callOpen < 0 {
			// Not a terminated call chain on this line: the legacy path
			// produces the story 05 errors (`a.b` without a call, safe
			// tails, unbalanced chains).
			return lp.parseNavigationCall(tokens, line)
		}
	}
	if !callClosesOnLine(tokens, callOpen) {
		return lp.parseMultilineCallArguments(tokens, callOpen, line, segments)
	}
	next, inner, err := consumeCall(tokens, callOpen)
	if err != nil {
		return Statement{}, err
	}
	if next != len(tokens) {
		return Statement{}, newError(UnsupportedSyntax, tokens[next].start, "unexpected token after call")
	}
	args, aerr := parseCallArgumentList(inner, line)
	if aerr != nil {
		return Statement{}, aerr
	}
	if isPunct(tokens[1], "(") && IntrinsicNames[tokens[0].text] && len(args) != 1 {
		// Story 41 (RFC-007 §6.6.5): the assertion shape takes exactly
		// one operand.
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, tokens[0].text+" requires exactly one argument")
	}
	lp.pos++
	return Statement{
		Kind:   Call,
		Call:   &NavigationExpr{Receiver: tokens[0].text, ReceiverSpan: Span{Start: tokens[0].start, End: tokens[0].end}, Segments: segments},
		Values: args,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// callChainScan parses the `identifier { (`. | `?.` identifier }` chain of a
// navigation call and reports the index of the call's opening parenthesis.
// callOpen < 0 means the line does not carry a terminated or unterminated
// call chain and the caller falls back to the legacy navigation path.
func callChainScan(tokens []token) ([]NavigationSegment, int, *Error) {
	segments := []NavigationSegment{}
	pos := 1
	safeTail := false
	for {
		if pos >= len(tokens) {
			return segments, -1, nil
		}
		safe := false
		switch {
		case isPunct(tokens[pos], "?"):
			if pos+1 >= len(tokens) || !isPunct(tokens[pos+1], ".") {
				return segments, -1, nil
			}
			safe = true
			pos += 2
		case isPunct(tokens[pos], "."):
			if safeTail {
				return nil, -1, newError(UnsupportedSyntax, tokens[pos].start, "ordinary selector cannot follow safe navigation")
			}
			pos++
		default:
			return segments, -1, nil
		}
		if pos >= len(tokens) || tokens[pos].kind != tokenIdent || reservedWords[tokens[pos].text] {
			return nil, -1, newError(UnsupportedSyntax, tokens[pos].start, "navigation operator requires a member name")
		}
		member := tokens[pos]
		segments = append(segments, NavigationSegment{
			Name: member.text,
			Safe: safe,
			Span: Span{Start: tokens[pos-1].start, End: member.end},
		})
		if safe {
			safeTail = true
		}
		pos++
		if pos < len(tokens) && isPunct(tokens[pos], "(") {
			// A call on a safe tail is a valid statement (story 08 Q3-A,
			// RFC-002 §33/§42); ordinary-after-safe stays rejected above.
			segments[len(segments)-1].Call = true
			return segments, pos, nil
		}
		if len(segments) > 0 {
			segments[len(segments)-1].Call = false
		}
	}
}

// callClosesOnLine reports whether the call opened at callOpen closes within
// the same token slice (parenthesis balancing).
func callClosesOnLine(tokens []token, callOpen int) bool {
	depth := 0
	for i := callOpen; i < len(tokens); i++ {
		if isPunct(tokens[i], "(") {
			depth++
		} else if isPunct(tokens[i], ")") {
			depth--
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// parseNavigationCall is the story 05 legacy path for lines that are not a
// terminated call chain: it keeps the original errors and rejects.
func (lp *lineParser) parseNavigationCall(tokens []token, line sourceLine) (Statement, error) {
	navigation, _, recognized, navErr := navigationMetadata(tokens)
	if navErr != nil {
		return Statement{}, navErr
	}
	if navigation == nil || len(navigation.Segments) == 0 {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "call statement must end with an ordinary call")
	}
	last := navigation.Segments[len(navigation.Segments)-1]
	if !recognized || !last.Call || last.Safe {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "call statement must end with an ordinary call")
	}
	if !isPunct(tokens[len(tokens)-1], ")") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "unexpected token after call")
	}
	if !isPunct(tokens[len(tokens)-2], "(") {
		return Statement{}, newError(UnsupportedSyntax, tokens[len(tokens)-2].start, "call statements are zero-argument")
	}
	lp.pos++
	return Statement{
		Kind: Call,
		Call: navigation,
		Span: Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseCallArgumentList parses the comma-separated argument values from the
// tokens between a call's parentheses. Empty parentheses carry no arguments.
func parseCallArgumentList(inner []token, line sourceLine) ([]Value, error) {
	if len(inner) == 0 {
		return nil, nil
	}
	return parseValueList(inner, 0, line)
}

// parseMultilineCallArguments parses a call whose argument list continues on
// following lines. Story 07 supports exactly the RFC-003 §41 shape: the
// first argument is a closure whose header closes on the statement line, and
// the remaining arguments continue after the block closer line and end with
// the call's `)`.
func (lp *lineParser) parseMultilineCallArguments(tokens []token, callOpen int, line sourceLine, segments []NavigationSegment) (Statement, error) {
	fi := callOpen + 1
	if fi+1 >= len(tokens) || tokens[fi].kind != tokenIdent || tokens[fi].text != "func" || !isPunct(tokens[fi+1], "(") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "unterminated navigation call")
	}
	closeIdx, params, perr := parseClosureParamGroups(tokens, fi+1, line)
	if perr != nil {
		return Statement{}, perr
	}
	if closeIdx != len(tokens)-1 || !isPunct(tokens[len(tokens)-1], "{") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "closure requires a block")
	}
	cl := Closure{Params: params, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	lp.pos++
	// A closure body is a function boundary: loop depth does not carry in.
	savedDepth := lp.loopDepth
	lp.loopDepth = 0
	body, suffix, closerLine, berr := lp.parseBlockWithSuffix()
	lp.loopDepth = savedDepth
	if berr != nil {
		return Statement{}, berr
	}
	cl.Body = body
	args := []Value{{Closure: &cl}}
	rest := suffix
	if len(rest) > 0 && isPunct(rest[0], ",") {
		if len(rest) < 2 || !isPunct(rest[len(rest)-1], ")") {
			return Statement{}, newError(UnsupportedSyntax, closerLine.offset, "unterminated navigation call")
		}
		more, verr := parseValueList(rest[1:len(rest)-1], 0, closerLine)
		if verr != nil {
			return Statement{}, verr
		}
		args = append(args, more...)
		rest = rest[len(rest)-1:]
	}
	if len(rest) != 1 || !isPunct(rest[0], ")") {
		return Statement{}, newError(UnsupportedSyntax, closerLine.offset, "unterminated navigation call")
	}
	return Statement{
		Kind:   Call,
		Call:   &NavigationExpr{Receiver: tokens[0].text, ReceiverSpan: Span{Start: tokens[0].start, End: tokens[0].end}, Segments: segments},
		Values: args,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseIf parses `if <expression> {` plus its block and an optional
// `} else {` block. The condition expression is kept as raw source text.
func (lp *lineParser) parseIf(tokens []token, line sourceLine) (Statement, error) {
	last := tokens[len(tokens)-1]
	if len(tokens) < 3 || last.kind != tokenPunct || last.text != "{" {
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "if requires a block")
	}
	condition := tokens[1 : len(tokens)-1]
	if condition[0].kind == tokenIdent && condition[0].text == "func" {
		return Statement{}, newError(UnsupportedSyntax, condition[0].start, "closure literal is not allowed in a condition")
	}
	if conditionErr := validateConditionTokens(condition); conditionErr != nil {
		return Statement{}, conditionErr
	}
	navigation, condIdents, recognized, navErr := navigationMetadata(condition)
	if navErr != nil {
		return Statement{}, navErr
	}
	if !recognized {
		if validationErr := validateNavigationTokens(condition); validationErr != nil {
			return Statement{}, validationErr
		}
		var identErr *Error
		condIdents, identErr = valueIdents(condition)
		if identErr != nil {
			return Statement{}, identErr
		}
	}
	statement := Statement{
		Kind:           If,
		Cond:           line.raw[condition[0].start-line.offset : condition[len(condition)-1].end-line.offset],
		CondIdents:     condIdents,
		CondNavigation: navigation,
		Span:           Span{Start: line.offset, End: line.offset + len(line.text)},
	}
	lp.pos++
	body, err := lp.parseBlock()
	if err != nil {
		return Statement{}, err
	}
	statement.Body = body
	if lp.pos < len(lp.lines) {
		next := lp.lines[lp.pos]
		switch {
		case lp.isElseHeader(next):
			lp.pos++
			elseBody, err := lp.parseBlock()
			if err != nil {
				return Statement{}, err
			}
			statement.Else = elseBody
		case lp.isElseIfHeader(next):
			// D-03: `} else if` is parse-level sugar for a nested if
			// statement in Else; the kernel CFG is unchanged. parseIf
			// recurses, so chains and a final `} else {` all work.
			tokens2, terr := tokenize(next.raw, next.offset)
			if terr != nil {
				return Statement{}, terr
			}
			inner, err := lp.parseIf(tokens2[2:], next)
			if err != nil {
				return Statement{}, err
			}
			inner.Span.Start = tokens2[2].start
			statement.Else = []Statement{inner}
		}
	}
	return statement, nil
}

// parseLoop parses the three confirmed loop forms: `for [Expression] Block`
// (condition / infinite) and `for identifier in Expression Block` (iteration).
// The header is one line closed by `{`; condition and collection are kept as
// raw source text. Iteration form stores the binding in Names and the
// collection expression in Values[0].
func (lp *lineParser) parseLoop(tokens []token, line sourceLine) (Statement, error) {
	last := tokens[len(tokens)-1]
	if len(tokens) < 2 || last.kind != tokenPunct || last.text != "{" {
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "loop requires a block")
	}
	statement := Statement{Kind: Loop, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	header := tokens[1 : len(tokens)-1]
	if len(header) >= 2 &&
		header[0].kind == tokenIdent && !reservedWords[header[0].text] &&
		header[1].kind == tokenIdent && header[1].text == "in" {
		if len(header) == 2 {
			return Statement{}, newError(UnsupportedSyntax, last.start, "iteration requires an expression")
		}
		collection := header[2:]
		if collection[0].kind == tokenIdent && collection[0].text == "func" {
			return Statement{}, newError(UnsupportedSyntax, collection[0].start, "closure literal is not allowed in a condition")
		}
		navigation, idents, metaErr := expressionMetadata(collection)
		if metaErr != nil {
			return Statement{}, metaErr
		}
		statement.Names = []string{header[0].text}
		statement.Values = []Value{{
			Text:       line.raw[collection[0].start-line.offset : collection[len(collection)-1].end-line.offset],
			Idents:     idents,
			Navigation: navigation,
		}}
	} else if len(header) > 0 {
		if header[0].kind == tokenIdent && header[0].text == "func" {
			return Statement{}, newError(UnsupportedSyntax, header[0].start, "closure literal is not allowed in a condition")
		}
		if condErr := validateConditionTokens(header); condErr != nil {
			return Statement{}, condErr
		}
		navigation, condIdents, metaErr := expressionMetadata(header)
		if metaErr != nil {
			return Statement{}, metaErr
		}
		statement.Cond = line.raw[header[0].start-line.offset : header[len(header)-1].end-line.offset]
		statement.CondIdents = condIdents
		statement.CondNavigation = navigation
	}
	lp.pos++
	lp.loopDepth++
	body, err := lp.parseBlock()
	lp.loopDepth--
	if err != nil {
		return Statement{}, err
	}
	statement.Body = body
	return statement, nil
}

// parseJump parses a bare `break` or `continue` inside a loop. The jump
// binds to the nearest enclosing loop; labels are deferred, and the loop
// depth resets at closure literals, so a jump never crosses a function
// boundary.
func (lp *lineParser) parseJump(tokens []token, line sourceLine) (Statement, error) {
	jump := tokens[0].text
	if len(tokens) > 1 {
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, jump+" accepts no label")
	}
	if lp.loopDepth == 0 {
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, jump+" outside loop")
	}
	lp.pos++
	kind := Break
	if jump == "continue" {
		kind = Continue
	}
	return Statement{Kind: kind, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
}

// expressionMetadata mirrors the if-condition pipeline: navigation metadata
// when the expression is a recognized selector chain, otherwise validated raw
// identifier lists.
func expressionMetadata(tokens []token) (*NavigationExpr, []string, *Error) {
	navigation, idents, recognized, navErr := navigationMetadata(tokens)
	if navErr != nil {
		return nil, nil, navErr
	}
	if !recognized {
		if validationErr := validateNavigationTokens(tokens); validationErr != nil {
			return nil, nil, validationErr
		}
		var identErr *Error
		idents, identErr = valueIdents(tokens)
		if identErr != nil {
			return nil, nil, identErr
		}
	}
	return navigation, idents, nil
}

func (lp *lineParser) isElseHeader(line sourceLine) bool {
	tokens, terr := tokenize(line.raw, line.offset)
	if terr != nil {
		return false
	}
	return len(tokens) == 3 &&
		tokens[0].kind == tokenPunct && tokens[0].text == "}" &&
		tokens[1].kind == tokenIdent && tokens[1].text == "else" &&
		tokens[2].kind == tokenPunct && tokens[2].text == "{"
}

type tokenKind uint8

const (
	tokenIdent tokenKind = iota
	tokenInt
	tokenString
	tokenBlank
	tokenPunct
)

type token struct {
	kind       tokenKind
	text       string
	start, end int // absolute byte offsets
}

var reservedWords = map[string]bool{"var": true, "if": true, "else": true, "for": true, "break": true, "continue": true, "in": true, "nil": true, "true": true, "false": true, "return": true, "type": true, "interface": true, "impl": true, "unsafe": true}

// IntrinsicNames reserves the compiler-intrinsic namespace (story 41,
// RFC-007 §6.6.13): a declaration with one of these names is a parse
// reject, while call-shaped uses route to the kernel's intrinsic
// handling instead of ordinary name resolution.
var IntrinsicNames = map[string]bool{"assume_non_nil": true}

func isReservedName(name string) bool {
	return reservedWords[name] || IntrinsicNames[name]
}

// isStatementKeyword reports words rejected in expression and condition
// positions. `in` is contextual: it is only recognized in a loop header
// between the binding and the collection expression.
func isStatementKeyword(text string) bool {
	return text == "var" || text == "if" || text == "else" || text == "for" ||
		text == "break" || text == "continue" || text == "in"
}

func isIdentStart(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || ('0' <= c && c <= '9')
}

func tokenize(line string, base int) ([]token, *Error) {
	var tokens []token
	i := 0
	for i < len(line) {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case isIdentStart(c):
			j := i + 1
			for j < len(line) && isIdentPart(line[j]) {
				j++
			}
			text := line[i:j]
			kind := tokenIdent
			if text == "_" {
				kind = tokenBlank
			}
			tokens = append(tokens, token{kind: kind, text: text, start: base + i, end: base + j})
			i = j
		case '0' <= c && c <= '9':
			j := i
			for j < len(line) && '0' <= line[j] && line[j] <= '9' {
				j++
			}
			tokens = append(tokens, token{kind: tokenInt, text: line[i:j], start: base + i, end: base + j})
			i = j
		case c == '"':
			j := i + 1
			for j < len(line) && line[j] != '"' {
				j++
			}
			if j >= len(line) {
				return nil, newError(UnsupportedSyntax, base+i, "unterminated string literal")
			}
			j++
			tokens = append(tokens, token{kind: tokenString, text: line[i:j], start: base + i, end: base + j})
			i = j
		case c == ':':
			if i+1 < len(line) && line[i+1] == '=' {
				return nil, newError(ShortDeclaration, base+i, ":= is a syntax error")
			}
			tokens = append(tokens, token{kind: tokenPunct, text: string(c), start: base + i, end: base + i + 1})
			i++
		case c == '!':
			if i+1 < len(line) && line[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenPunct, text: "!=", start: base + i, end: base + i + 2})
				i += 2
				continue
			}
			return nil, newError(UnsupportedSyntax, base+i, "force unwrap is not part of the language")
		case strings.ContainsRune("=,.?()[]*+-{}", rune(c)):
			if c == '=' && i+1 < len(line) && line[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenPunct, text: "==", start: base + i, end: base + i + 2})
				i += 2
				continue
			}
			tokens = append(tokens, token{kind: tokenPunct, text: string(c), start: base + i, end: base + i + 1})
			i++
		default:
			return nil, newError(UnsupportedSyntax, base+i, fmt.Sprintf("unexpected character %q", c))
		}
	}
	return tokens, nil
}

func isAssign(t token) bool { return t.kind == tokenPunct && t.text == "=" }

func listEnd(tokens []token) int { return tokens[len(tokens)-1].end }

// isClosureStart reports whether the value starting at index i is a closure
// literal: the token sequence `func (` at the start of the value.
func isClosureStart(tokens []token, i int) bool {
	return i+1 < len(tokens) &&
		tokens[i].kind == tokenIdent && tokens[i].text == "func" &&
		tokens[i+1].kind == tokenPunct && tokens[i+1].text == "("
}

func parseNameList(tokens []token, start int) ([]string, int, *Error) {
	var names []string
	i := start
	for {
		if i >= len(tokens) {
			return nil, 0, newError(UnsupportedSyntax, listEnd(tokens), "expected identifier")
		}
		if tokens[i].kind != tokenIdent && tokens[i].kind != tokenBlank {
			return nil, 0, newError(UnsupportedSyntax, tokens[i].start, "expected identifier")
		}
		if IntrinsicNames[tokens[i].text] {
			// Story 41 (RFC-007 §6.6.13): the intrinsic namespace is
			// reserved - bindings cannot shadow compiler intrinsics.
			return nil, 0, newError(UnsupportedSyntax, tokens[i].start, tokens[i].text+" is a reserved intrinsic name")
		}
		// GB-3 variant A: `_` is accepted in name lists as a write-only
		// discard; it stays in the name list for arity but the kernel
		// creates no binding for it.
		names = append(names, tokens[i].text)
		i++
		if i < len(tokens) && tokens[i].kind == tokenPunct && tokens[i].text == "," {
			i++
			continue
		}
		return names, i, nil
	}
}

type typeParser struct {
	tokens []token
	pos    int
}

func parseType(tokens []token) (*TypeExpr, *Error) {
	p := typeParser{tokens: tokens}
	typ, err := p.parseType()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.tokens) {
		return nil, p.errorAtCurrent("invalid type expression")
	}
	return typ, nil
}

func (p *typeParser) parseType() (*TypeExpr, *Error) {
	typ, err := p.parseOperand()
	if err != nil {
		return nil, err
	}
	if p.take("?") {
		typ.Nullable = true
		typ.Span.End = p.tokens[p.pos-1].end
	}
	return typ, nil
}

func (p *typeParser) parseOperand() (*TypeExpr, *Error) {
	if p.pos >= len(p.tokens) {
		return nil, p.errorAtCurrent("expected type operand")
	}
	t := p.tokens[p.pos]
	switch {
	case t.kind == tokenIdent && t.text == "map" && p.peek("["):
		p.pos++
		p.pos++
		key, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		if !p.take("]") {
			return nil, p.errorAtCurrent("map type requires closing ]")
		}
		value, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		return &TypeExpr{Kind: MapType, Key: key, Value: value, Span: Span{Start: t.start, End: value.Span.End}}, nil
	case t.kind == tokenIdent:
		p.pos++
		return &TypeExpr{Kind: NamedType, Name: t.text, Span: Span{Start: t.start, End: t.end}}, nil
	case t.kind == tokenPunct && t.text == "*":
		p.pos++
		elem, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		return &TypeExpr{Kind: PointerType, Elem: elem, Span: Span{Start: t.start, End: elem.Span.End}}, nil
	case t.kind == tokenPunct && t.text == "[":
		p.pos++
		if !p.take("]") {
			return nil, p.errorAtCurrent("slice type requires closing ]")
		}
		elem, err := p.parseOperand()
		if err != nil {
			return nil, err
		}
		return &TypeExpr{Kind: SliceType, Elem: elem, Span: Span{Start: t.start, End: elem.Span.End}}, nil
	case t.kind == tokenPunct && t.text == "(":
		p.pos++
		inner, err := p.parseType()
		if err != nil {
			return nil, err
		}
		if !p.take(")") {
			return nil, p.errorAtCurrent("parenthesized type requires closing )")
		}
		inner.Span = Span{Start: t.start, End: p.tokens[p.pos-1].end}
		return inner, nil
	default:
		return nil, p.errorAtCurrent("expected type operand")
	}
}

func (p *typeParser) take(text string) bool {
	if p.pos >= len(p.tokens) || p.tokens[p.pos].kind != tokenPunct || p.tokens[p.pos].text != text {
		return false
	}
	p.pos++
	return true
}

func (p *typeParser) peek(text string) bool {
	return p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].kind == tokenPunct && p.tokens[p.pos+1].text == text
}

func (p *typeParser) errorAtCurrent(message string) *Error {
	offset := 0
	if p.pos < len(p.tokens) {
		offset = p.tokens[p.pos].start
	} else if len(p.tokens) > 0 {
		offset = p.tokens[len(p.tokens)-1].end
	}
	return newError(UnsupportedSyntax, offset, message)
}

func (lp *lineParser) parseVar(tokens []token, line sourceLine) (Statement, error) {
	names, i, terr := parseNameList(tokens, 1)
	if terr != nil {
		return Statement{}, terr
	}
	for _, name := range names {
		if reservedWords[name] || IntrinsicNames[name] {
			// Story 41: keywords (`unsafe`) and the reserved intrinsic
			// namespace (§6.6.13) cannot name bindings - the func-decl
			// precedent.
			return Statement{}, newError(UnsupportedSyntax, tokens[1].start, name+" is a reserved name")
		}
	}
	statement := Statement{Kind: Var, Names: names, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	typeStart := i
	for i < len(tokens) && !isAssign(tokens[i]) {
		i++
	}
	if len(names) > 1 && typeStart < i {
		return Statement{}, newError(TypedMultipleDeclaration, tokens[typeStart].start, "typed multiple declaration is excluded from the confirmed grammar")
	}
	if typeStart < i {
		typeExpr, typeErr := parseType(tokens[typeStart:i])
		if typeErr != nil {
			return Statement{}, typeErr
		}
		statement.Type = line.raw[tokens[typeStart].start-line.offset : tokens[i-1].end-line.offset]
		statement.TypeExpr = typeExpr
	} else if i >= len(tokens) {
		return Statement{}, newError(BareDeclaration, tokens[0].start, "declaration requires a type or an initializer")
	}
	if i < len(tokens) {
		if isClosureStart(tokens, i+1) {
			if len(names) != 1 {
				return Statement{}, newError(UnsupportedSyntax, tokens[i+1].start, "closure initializer requires a single binding")
			}
			cl, cerr := lp.parseClosure(tokens, i+1, line, false)
			if cerr != nil {
				return Statement{}, cerr
			}
			// lp.pos was advanced past the closure body by parseClosure.
			statement.Values = []Value{{Closure: &cl}}
			return statement, nil
		}
		if len(tokens) > i+2 && tokens[i+1].kind == tokenIdent && tokens[i+1].text == "try" && tokens[i+2].kind == tokenIdent {
			// Story 35 (RFC-005 §6.5.1–6.5.2): the value-try declaration
			// `var v = try F(x)` - Names receive the success results on the
			// success path; the trailing `error?` propagates. Fallibility
			// of the enclosing function and of the callee is a kernel
			// check (ANUY6001, D-2). parseCallStatement advances lp.pos
			// past this line.
			callStatement, cerr := lp.parseCallStatement(tokens[i+2:], line)
			if cerr != nil {
				return Statement{}, cerr
			}
			statement.TryCall = callStatement.Call
			statement.Values = callStatement.Values
			return statement, nil
		}
		values, verr := lp.parseRHS(tokens, i+1, line)
		if verr != nil {
			return Statement{}, verr
		}
		statement.Values = values
	}
	lp.pos++
	return statement, nil
}

func (lp *lineParser) parseAssign(tokens []token, line sourceLine) (Statement, error) {
	names, i, terr := parseNameList(tokens, 0)
	if terr != nil {
		return Statement{}, terr
	}
	if i >= len(tokens) || !isAssign(tokens[i]) {
		if offset, hasSafeTarget := safeNavigationTargetOffset(tokens, i); hasSafeTarget {
			return Statement{}, newError(UnsupportedSyntax, offset, "safe navigation is not an assignment target")
		}
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "assignment requires =")
	}
	var values []Value
	if isClosureStart(tokens, i+1) {
		if len(names) != 1 {
			return Statement{}, newError(UnsupportedSyntax, tokens[i+1].start, "closure initializer requires a single binding")
		}
		cl, cerr := lp.parseClosure(tokens, i+1, line, false)
		if cerr != nil {
			return Statement{}, cerr
		}
		// lp.pos was advanced past the closure body by parseClosure.
		values = []Value{{Closure: &cl}}
	} else {
		rest, verr := lp.parseRHS(tokens, i+1, line)
		if verr != nil {
			return Statement{}, verr
		}
		values = rest
	}
	if len(values) > 1 && len(values) != len(names) {
		return Statement{}, newError(ArityMismatch, tokens[0].start, "assignment arity mismatch")
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if name == "_" {
			// GB-3 variant A: the blank identifier is not a binding target;
			// the duplicate-target check (RFC-003 §63) does not apply.
			continue
		}
		if seen[name] {
			return Statement{}, newError(DuplicateAssignmentTarget, tokens[0].start, fmt.Sprintf("duplicate assignment target %q", name))
		}
		seen[name] = true
	}
	if len(values) == 1 && values[0].Closure != nil {
		// lp.pos was already advanced past the closure body.
		return Statement{
			Kind:   Assign,
			Names:  names,
			Values: values,
			Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
		}, nil
	}
	lp.pos++
	return Statement{
		Kind:   Assign,
		Names:  names,
		Values: values,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// hasTopLevelAssign reports whether the line carries an `=` outside any
// brackets/braces (the `==` token is separate and does not count).
func hasTopLevelAssign(tokens []token) bool {
	depth := 0
	for _, t := range tokens {
		switch {
		case isPunct(t, "(") || isPunct(t, "[") || isPunct(t, "{"):
			depth++
		case isPunct(t, ")") || isPunct(t, "]") || isPunct(t, "}"):
			depth--
		case depth == 0 && isAssign(t):
			return true
		}
	}
	return false
}

// parseFieldAssignment parses the field-mutation forms `u.f = expr`
// (story 22) and `u.f.g = expr` (story 25, RFC-014 §6.7): an ordinary
// path as the assignment target. Deeper paths reject explicitly; a safe
// segment in the target is not an assignment target.
func (lp *lineParser) parseFieldAssignment(tokens []token, line sourceLine) (Statement, error) {
	if tokens[0].text == "_" || reservedWords[tokens[0].text] {
		return Statement{}, newError(UnsupportedSyntax, tokens[0].start, "invalid assignment target")
	}
	eq := -1
	for i := 1; i < len(tokens); i++ {
		if isAssign(tokens[i]) {
			eq = i
			break
		}
	}
	if eq < 0 {
		if offset, hasSafeTarget := safeNavigationTargetOffset(tokens, 1); hasSafeTarget {
			return Statement{}, newError(UnsupportedSyntax, offset, "safe navigation is not an assignment target")
		}
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "assignment requires =")
	}
	for _, t := range tokens[1:eq] {
		if isPunct(t, "?") {
			return Statement{}, newError(UnsupportedSyntax, t.start, "safe navigation is not an assignment target")
		}
	}
	var names []token
	for i := 2; i < eq; i += 2 {
		if !isPunct(tokens[i-1], ".") || tokens[i].kind != tokenIdent || tokens[i].text == "_" || reservedWords[tokens[i].text] {
			return Statement{}, newError(UnsupportedSyntax, tokens[i].start, "field assignment requires a field name")
		}
		names = append(names, tokens[i])
	}
	if eq != 2*len(names)+1 {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "field assignment requires a field name")
	}
	if len(names) > 2 {
		return Statement{}, newError(UnsupportedSyntax, tokens[5].start, "deeper field paths are not supported in this slice")
	}
	segments := make([]NavigationSegment, 0, len(names))
	for i, name := range names {
		segments = append(segments, NavigationSegment{
			Name: name.text,
			Span: Span{Start: tokens[1+2*i].start, End: name.end},
		})
	}
	values, verr := lp.parseRHS(tokens, eq+1, line)
	if verr != nil {
		return Statement{}, verr
	}
	lp.pos++
	return Statement{
		Kind: Assign,
		Target: &NavigationExpr{
			Receiver:     tokens[0].text,
			ReceiverSpan: Span{Start: tokens[0].start, End: tokens[0].end},
			Segments:     segments,
		},
		Values: values,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseClosure parses `func(Params) {` plus its block. lp.pos is advanced
// past every consumed line.
func (lp *lineParser) parseClosure(tokens []token, start int, line sourceLine, allowResult bool) (Closure, *Error) {
	i, params, err := parseClosureParamGroups(tokens, start+1, line)
	if err != nil {
		return Closure{}, err
	}
	hasResult, resultNullable := false, false
	var resultTypeExpr *TypeExpr
	var resultList []*TypeExpr
	resultCount := 0
	frameFallible := false
	if i < len(tokens) && !(tokens[i].kind == tokenPunct && tokens[i].text == "{") {
		// An optional result type before the block (story 08 Q4-A, RFC-002
		// §40 spelling: `func f() User? {`). Types carry no braces in this
		// grammar, so the block opener terminates the type. Story 35
		// (RFC-005 §6.2.3): a `(` opens a result list `(T1, …, Tn, error?)`.
		if !allowResult {
			return Closure{}, newError(UnsupportedSyntax, tokens[i].start, "closure literal takes no result type")
		}
		if tokens[i].kind == tokenPunct && tokens[i].text == "(" {
			list, closeIdx, listErr := parseResultList(tokens, i)
			if listErr != nil {
				return Closure{}, listErr
			}
			last := list[len(list)-1]
			hasResult, resultNullable = true, last.Nullable
			resultTypeExpr = last
			resultList = list
			resultCount = len(list)
			frameFallible = true
			i = closeIdx
		} else {
			j := i
			for j < len(tokens) && !(tokens[j].kind == tokenPunct && tokens[j].text == "{") {
				j++
			}
			if j >= len(tokens) {
				return Closure{}, newError(UnsupportedSyntax, listEnd(tokens), "closure requires a block")
			}
			typeExpr, typeErr := parseType(tokens[i:j])
			if typeErr != nil {
				return Closure{}, typeErr
			}
			hasResult, resultNullable = true, typeExpr.Nullable
			resultTypeExpr = typeExpr
			resultCount = 1
			frameFallible = typeExpr.Kind == NamedType && typeExpr.Name == "error" && typeExpr.Nullable
			i = j
		}
	}
	if i >= len(tokens) || tokens[i].kind != tokenPunct || tokens[i].text != "{" {
		return Closure{}, newError(UnsupportedSyntax, listEnd(tokens), "closure requires a block")
	}
	cl := Closure{Params: params, HasResult: hasResult, ResultNullable: resultNullable, ResultTypeExpr: resultTypeExpr, ResultList: resultList, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	lp.pos++
	// A closure body is a function boundary: loop depth does not carry in,
	// and the result-type frame scopes `return expr` (story 08).
	savedDepth := lp.loopDepth
	lp.loopDepth = 0
	lp.funcStack = append(lp.funcStack, returnFrame{hasResult: hasResult, results: resultCount, fallible: frameFallible})
	body, berr := lp.parseBlock()
	lp.funcStack = lp.funcStack[:len(lp.funcStack)-1]
	lp.loopDepth = savedDepth
	if berr != nil {
		return Closure{}, berr.(*Error)
	}
	cl.Body = body
	return cl, nil
}

// parseResultList parses the parenthesized result list `(T1, …, Tn)` whose
// opening `(` is at index open (story 35, RFC-005 §6.2.3). It returns the
// parsed types and the index just after the closing `)`. The slice gate
// accepts only the fallible shape - the trailing element must be `error?`
// (§6.2.4: unusual shapes like `(error?, int)` are not fallible signatures,
// and non-fallible multi-result functions are RFC-003 work).
func parseResultList(tokens []token, open int) ([]*TypeExpr, int, *Error) {
	depth := 0
	closeIdx := -1
	for i := open; i < len(tokens); i++ {
		t := tokens[i]
		if t.kind != tokenPunct {
			continue
		}
		if t.text == "(" {
			depth++
			continue
		}
		if t.text == ")" {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx < 0 {
		return nil, 0, newError(UnsupportedSyntax, listEnd(tokens), "unterminated result list")
	}
	var groups [][]token
	cur := []token{}
	depth = 1 // reuse the scanner depth, now inside the opening parenthesis
	for i := open + 1; i < closeIdx; i++ {
		t := tokens[i]
		if t.kind == tokenPunct && t.text == "," && depth == 1 {
			groups = append(groups, cur)
			cur = []token{}
			continue
		}
		if t.kind == tokenPunct && (t.text == "(" || t.text == "[") {
			depth++
		}
		if t.kind == tokenPunct && (t.text == ")" || t.text == "]") {
			depth--
		}
		cur = append(cur, t)
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	if len(groups) < 2 {
		return nil, 0, newError(UnsupportedSyntax, tokens[open].start, "result list requires at least two results in this slice")
	}
	var list []*TypeExpr
	for _, g := range groups {
		typeExpr, typeErr := parseType(g)
		if typeErr != nil {
			return nil, 0, typeErr
		}
		list = append(list, typeExpr)
	}
	last := list[len(list)-1]
	if last.Kind != NamedType || last.Name != "error" || !last.Nullable {
		return nil, 0, newError(UnsupportedSyntax, last.Span.Start, "result list requires a trailing error? in this slice")
	}
	return list, closeIdx + 1, nil
}

// parseInterfaceDecl parses `interface Name {` plus the signature block
// (story 39, RFC-004 §6.1.1): each body line is `name(params) [result]`,
// the `}` line closes. Zero-method interfaces are valid; duplicate method
// names reject at the parse level (§6.1.6 structural).
func (lp *lineParser) parseInterfaceDecl(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) < 2 || tokens[1].kind != tokenIdent || tokens[1].text == "_" || reservedWords[tokens[1].text] {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "invalid interface name")
	}
	if len(tokens) < 3 || !isPunct(tokens[2], "{") {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "interface declaration requires `{`")
	}
	decl := &InterfaceDecl{Name: tokens[1].text, Span: Span{Start: tokens[0].start}}
	seen := map[string]bool{}
	lp.pos++
	for {
		if lp.pos >= len(lp.lines) {
			return Statement{}, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		sigLine := lp.lines[lp.pos]
		if sigLine.text == "" || strings.HasPrefix(sigLine.text, "//") {
			lp.pos++
			continue
		}
		if sigLine.text == "}" {
			lp.pos++
			decl.Span.End = sigLine.offset + len(sigLine.text)
			return Statement{Kind: Interface, Names: []string{decl.Name}, Interface: decl, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
		}
		sigTokens, terr := tokenize(sigLine.raw, sigLine.offset)
		if terr != nil {
			return Statement{}, terr
		}
		method, merr := parseInterfaceMethod(sigTokens, sigLine)
		if merr != nil {
			return Statement{}, merr
		}
		if seen[method.Name] {
			return Statement{}, newError(UnsupportedSyntax, method.Span.Start, "duplicate interface method "+method.Name)
		}
		seen[method.Name] = true
		decl.Methods = append(decl.Methods, method)
		lp.pos++
	}
}

// parseInterfaceMethod parses one signature line `name(params) [result]`.
func parseInterfaceMethod(tokens []token, line sourceLine) (InterfaceMethod, *Error) {
	if len(tokens) < 2 || tokens[0].kind != tokenIdent || tokens[0].text == "_" || reservedWords[tokens[0].text] {
		return InterfaceMethod{}, newError(UnsupportedSyntax, listEnd(tokens), "interface method requires a name")
	}
	if !isPunct(tokens[1], "(") {
		return InterfaceMethod{}, newError(UnsupportedSyntax, tokens[1].start, "interface method requires a parameter list")
	}
	i, params, err := parseClosureParamGroups(tokens, 1, line)
	if err != nil {
		return InterfaceMethod{}, err
	}
	m := InterfaceMethod{Name: tokens[0].text, Params: params, Span: Span{Start: tokens[0].start, End: listEnd(tokens)}}
	if i < len(tokens) {
		typeExpr, typeErr := parseType(tokens[i:])
		if typeErr != nil {
			return InterfaceMethod{}, typeErr
		}
		m.HasResult = true
		m.ResultNullable = typeExpr.Nullable
		m.ResultTypeExpr = typeExpr
	}
	return m, nil
}

// parseImplDecl parses the conformance statement `impl Iface for [*]Type`
// (story 39, RFC-004 §6.1.3): no body - the statement only asserts that
// the target type's method set satisfies the interface.
func (lp *lineParser) parseImplDecl(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) < 2 || tokens[1].kind != tokenIdent || tokens[1].text == "_" || reservedWords[tokens[1].text] {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "impl requires `impl Iface for Type`")
	}
	if len(tokens) < 4 || tokens[2].kind != tokenIdent || tokens[2].text != "for" {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "impl requires `impl Iface for Type`")
	}
	pointer := false
	ti := 3
	if isPunct(tokens[ti], "*") {
		pointer = true
		ti++
	}
	if ti != len(tokens)-1 || tokens[ti].kind != tokenIdent || tokens[ti].text == "_" || reservedWords[tokens[ti].text] {
		return Statement{}, newError(UnsupportedSyntax, listEnd(tokens), "impl target must be a named type")
	}
	decl := &ImplDecl{Interface: tokens[1].text, Type: tokens[ti].text, Pointer: pointer, Span: Span{Start: tokens[0].start, End: listEnd(tokens)}}
	lp.pos++
	return Statement{Kind: Impl, Names: []string{tokens[1].text}, Impl: decl, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}, nil
}

// parseClosureParamGroups parses the parenthesized parameter list of a
// closure literal whose opening `(` is at index start. It returns the index
// just after the closing `)` and the parameters in declaration order.
func parseClosureParamGroups(tokens []token, start int, line sourceLine) (int, []Param, *Error) {
	depth := 1
	i := start + 1
	var groups [][]token
	cur := []token{}
	for ; ; i++ {
		if i >= len(tokens) {
			return 0, nil, newError(UnsupportedSyntax, listEnd(tokens), "unterminated closure parameter list")
		}
		t := tokens[i]
		if t.kind == tokenPunct && (t.text == "(" || t.text == "[") {
			depth++
			cur = append(cur, t)
			continue
		}
		if t.kind == tokenPunct && (t.text == ")" || t.text == "]") {
			depth--
			if depth == 0 {
				if len(cur) > 0 {
					groups = append(groups, cur)
				}
				i++
				break
			}
			cur = append(cur, t)
			continue
		}
		if t.kind == tokenPunct && t.text == "," && depth == 1 {
			groups = append(groups, cur)
			cur = []token{}
			continue
		}
		cur = append(cur, t)
	}
	var params []Param
	for _, g := range groups {
		if g[0].kind != tokenIdent || g[0].text == "_" || reservedWords[g[0].text] {
			return 0, nil, newError(UnsupportedSyntax, g[0].start, "expected parameter name")
		}
		if len(g) < 2 {
			return 0, nil, newError(UnsupportedSyntax, g[0].start, "closure parameter requires a type")
		}
		typeExpr, typeErr := parseType(g[1:])
		if typeErr != nil {
			return 0, nil, typeErr
		}
		params = append(params, Param{
			Name:     g[0].text,
			Type:     line.raw[g[1].start-line.offset : g[len(g)-1].end-line.offset],
			TypeExpr: typeExpr,
		})
	}
	return i, params, nil
}

func isValueToken(t token) bool {
	switch t.kind {
	case tokenInt, tokenString, tokenIdent:
		return true
	case tokenPunct:
		return strings.ContainsRune("()[].?*+-{}:", rune(t.text[0])) || t.text == "==" || t.text == "!="
	default:
		return false
	}
}

func isPunct(t token, text string) bool {
	return t.kind == tokenPunct && t.text == text
}

func isCallCallee(t token) bool {
	return t.kind == tokenIdent || isPunct(t, ")") || isPunct(t, "]")
}

func validateConditionTokens(tokens []token) *Error {
	var delimiters []bool // true only for a call's opening parenthesis
	for i, t := range tokens {
		switch {
		case t.kind == tokenBlank:
			return newError(BlankIdentifierRead, t.start, "_ does not hold a value")
		case t.kind == tokenIdent && isStatementKeyword(t.text):
			return newError(UnsupportedSyntax, t.start, fmt.Sprintf("unexpected keyword %q in condition", t.text))
		case isPunct(t, "(") || isPunct(t, "["):
			isCall := isPunct(t, "(") && i > 0 && isCallCallee(tokens[i-1])
			delimiters = append(delimiters, isCall)
		case isPunct(t, ")") || isPunct(t, "]"):
			if len(delimiters) == 0 {
				return newError(UnsupportedSyntax, t.start, "unbalanced brackets")
			}
			delimiters = delimiters[:len(delimiters)-1]
		case isPunct(t, ","):
			if len(delimiters) > 0 && delimiters[len(delimiters)-1] {
				continue
			}
			return newError(UnsupportedSyntax, t.start, fmt.Sprintf("unexpected token %q in condition", t.text))
		case !isValueToken(t):
			return newError(UnsupportedSyntax, t.start, fmt.Sprintf("unexpected token %q in condition", t.text))
		}
	}
	if len(delimiters) != 0 {
		return newError(UnsupportedSyntax, listEnd(tokens), "unbalanced brackets")
	}
	return nil
}

// navigationMetadata recognizes a selector chain rooted at an identifier. A
// chain may have ordinary selectors before its first safe selector, but every
// selector after that point must remain safe (RFC-002 §49, owner-approved
// strict profile).
func navigationMetadata(tokens []token) (*NavigationExpr, []string, bool, *Error) {
	if len(tokens) == 0 {
		return nil, nil, false, nil
	}
	if isPunct(tokens[0], "(") {
		next, inner, err := consumeCall(tokens, 0)
		if err != nil {
			return nil, nil, false, err
		}
		innerNavigation, innerIdents, innerRecognized, innerErr := navigationMetadata(inner)
		if innerErr != nil {
			return nil, nil, false, innerErr
		}
		if !innerRecognized {
			if len(inner) != 1 || inner[0].kind != tokenIdent || reservedWords[inner[0].text] {
				return nil, nil, false, nil
			}
			innerNavigation = &NavigationExpr{Receiver: inner[0].text, ReceiverSpan: Span{Start: inner[0].start, End: inner[0].end}}
			innerIdents = []string{inner[0].text}
		}
		return navigationSuffix(tokens, next, innerNavigation.Receiver, innerNavigation.ReceiverSpan, innerNavigation.Segments, innerIdents)
	}
	if tokens[0].kind != tokenIdent || reservedWords[tokens[0].text] {
		return nil, nil, false, nil
	}
	idents := []string{tokens[0].text}
	pos := 1
	if pos < len(tokens) && isPunct(tokens[pos], "(") {
		next, args, err := consumeCall(tokens, pos)
		if err != nil {
			return nil, nil, false, err
		}
		argIdents, argErr := navigationArgumentIdents(args)
		if argErr != nil {
			return nil, nil, false, argErr
		}
		idents = append(idents, argIdents...)
		pos = next
	}
	return navigationSuffix(tokens, pos, tokens[0].text, Span{Start: tokens[0].start, End: tokens[0].end}, nil, idents)
}

func navigationSuffix(tokens []token, pos int, receiver string, receiverSpan Span, initial []NavigationSegment, idents []string) (*NavigationExpr, []string, bool, *Error) {
	segments := append([]NavigationSegment(nil), initial...)
	safeTail := false
	for _, segment := range segments {
		safeTail = safeTail || segment.Safe
	}
	for pos < len(tokens) {
		safe := false
		var operator token
		switch {
		case isPunct(tokens[pos], "?"):
			if pos+1 >= len(tokens) || !isPunct(tokens[pos+1], ".") {
				return nil, nil, false, nil
			}
			safe = true
			operator = tokens[pos]
			pos += 2
		case isPunct(tokens[pos], "."):
			operator = tokens[pos]
			if safeTail {
				return nil, nil, true, newError(UnsupportedSyntax, operator.start, "ordinary selector cannot follow safe navigation")
			}
			pos++
		default:
			return nil, nil, false, nil
		}
		if pos >= len(tokens) || tokens[pos].kind != tokenIdent || reservedWords[tokens[pos].text] {
			return nil, nil, true, newError(UnsupportedSyntax, operator.start, "navigation operator requires a member name")
		}
		member := tokens[pos]
		segments = append(segments, NavigationSegment{
			Name: member.text,
			Safe: safe,
			Span: Span{Start: operator.start, End: member.end},
		})
		if safe {
			safeTail = true
		}
		pos++
		if pos < len(tokens) && isPunct(tokens[pos], "(") {
			segments[len(segments)-1].Call = true
			next, args, err := consumeCall(tokens, pos)
			if err != nil {
				return nil, nil, false, err
			}
			argIdents, argErr := navigationArgumentIdents(args)
			if argErr != nil {
				return nil, nil, false, argErr
			}
			idents = append(idents, argIdents...)
			pos = next
		}
	}
	if len(segments) == 0 {
		return nil, nil, false, nil
	}
	return &NavigationExpr{Receiver: receiver, ReceiverSpan: receiverSpan, Segments: segments}, idents, true, nil
}

func validateNavigationTokens(tokens []token) *Error {
	for i, t := range tokens {
		if t.kind != tokenIdent && !isPunct(t, "(") {
			continue
		}
		_, _, _, err := navigationMetadata(tokens[i:])
		if err != nil {
			return err
		}
	}
	return nil
}

func consumeCall(tokens []token, start int) (int, []token, *Error) {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch {
		case isPunct(tokens[i], "(") || isPunct(tokens[i], "["):
			depth++
		case isPunct(tokens[i], ")") || isPunct(tokens[i], "]"):
			depth--
			if depth == 0 {
				return i + 1, tokens[start+1 : i], nil
			}
			if depth < 0 {
				return 0, nil, newError(UnsupportedSyntax, tokens[i].start, "unbalanced navigation call")
			}
		}
	}
	return 0, nil, newError(UnsupportedSyntax, tokens[start].start, "unterminated navigation call")
}

func navigationArgumentIdents(tokens []token) ([]string, *Error) {
	if len(tokens) == 0 {
		return nil, nil
	}
	var idents []string
	groupStart := 0
	depth := 0
	for i, t := range tokens {
		switch {
		case isPunct(t, "(") || isPunct(t, "["):
			depth++
		case isPunct(t, ")") || isPunct(t, "]"):
			depth--
			if depth < 0 {
				return nil, newError(UnsupportedSyntax, t.start, "unbalanced navigation arguments")
			}
		case isPunct(t, ",") && depth == 0:
			if i == groupStart {
				return nil, newError(UnsupportedSyntax, t.start, "missing navigation argument")
			}
			groupIdents, err := navigationArgumentGroupIdents(tokens[groupStart:i])
			if err != nil {
				return nil, err
			}
			idents = append(idents, groupIdents...)
			groupStart = i + 1
		}
	}
	if depth != 0 {
		return nil, newError(UnsupportedSyntax, tokens[len(tokens)-1].end, "unbalanced navigation arguments")
	}
	if groupStart == len(tokens) {
		return nil, newError(UnsupportedSyntax, tokens[len(tokens)-1].end, "missing navigation argument")
	}
	groupIdents, err := navigationArgumentGroupIdents(tokens[groupStart:])
	if err != nil {
		return nil, err
	}
	return append(idents, groupIdents...), nil
}

func navigationArgumentGroupIdents(tokens []token) ([]string, *Error) {
	_, idents, recognized, err := navigationMetadata(tokens)
	if err != nil {
		return nil, err
	}
	if recognized {
		return idents, nil
	}
	if validationErr := validateNavigationTokens(tokens); validationErr != nil {
		return nil, validationErr
	}
	return valueIdents(tokens)
}

func valueIdents(tokens []token) ([]string, *Error) {
	var idents []string
	for _, t := range tokens {
		if t.kind != tokenIdent {
			continue
		}
		if isStatementKeyword(t.text) {
			return nil, newError(UnsupportedSyntax, t.start, fmt.Sprintf("unexpected keyword %q in expression", t.text))
		}
		if !reservedWords[t.text] {
			idents = append(idents, t.text)
		}
	}
	return idents, nil
}

func safeNavigationTargetOffset(tokens []token, start int) (int, bool) {
	safeOffset := -1
	for i := start; i+1 < len(tokens); i++ {
		if isAssign(tokens[i]) {
			return safeOffset, safeOffset >= 0
		}
		if safeOffset < 0 && isPunct(tokens[i], "?") && isPunct(tokens[i+1], ".") {
			safeOffset = tokens[i].start
		}
	}
	return 0, false
}

// parseValueList splits the remaining tokens into top-level comma-separated
// expressions and keeps each expression as raw source text.
func parseValueList(tokens []token, start int, line sourceLine) ([]Value, error) {
	if start >= len(tokens) {
		return nil, newError(UnsupportedSyntax, listEnd(tokens), "missing expression")
	}
	var values []Value
	groupStart := start
	var delimiters []bool // true only for a call's opening parenthesis
	i := start
	for ; i < len(tokens); i++ {
		t := tokens[i]
		switch {
		case t.kind == tokenPunct && (t.text == "(" || t.text == "["):
			isCall := t.text == "(" && i > start && isCallCallee(tokens[i-1])
			delimiters = append(delimiters, isCall)
		case t.kind == tokenPunct && (t.text == ")" || t.text == "]"):
			if len(delimiters) == 0 {
				return nil, newError(UnsupportedSyntax, t.start, "unbalanced brackets")
			}
			delimiters = delimiters[:len(delimiters)-1]
			// Story 21: a keyed literal's braces guard its commas - the comma
			// inside is legal and never splits the value list.
		case t.kind == tokenPunct && t.text == "{":
			delimiters = append(delimiters, true)
		case t.kind == tokenPunct && t.text == "}":
			if len(delimiters) == 0 {
				return nil, newError(UnsupportedSyntax, t.start, "unbalanced brackets")
			}
			delimiters = delimiters[:len(delimiters)-1]
		case t.kind == tokenPunct && t.text == ",":
			if len(delimiters) > 0 {
				if delimiters[len(delimiters)-1] {
					continue
				}
				return nil, newError(UnsupportedSyntax, t.start, "unexpected comma in expression")
			}
			value, gerr := valueGroup(tokens, groupStart, i, line)
			if gerr != nil {
				return nil, gerr
			}
			values = append(values, value)
			groupStart = i + 1
		case t.kind == tokenPunct && (t.text == "=" || t.text == "=="):
			return nil, newError(UnsupportedSyntax, t.start, "unexpected = in expression")
		case t.kind == tokenBlank:
			return nil, newError(BlankIdentifierRead, t.start, "_ does not hold a value")
		case !isValueToken(t):
			return nil, newError(UnsupportedSyntax, t.start, fmt.Sprintf("unexpected token %q in expression", t.text))
		}
	}
	if len(delimiters) != 0 {
		return nil, newError(UnsupportedSyntax, listEnd(tokens), "unbalanced brackets")
	}
	value, gerr := valueGroup(tokens, groupStart, i, line)
	if gerr != nil {
		return nil, gerr
	}
	values = append(values, value)
	return values, nil
}

func valueGroup(tokens []token, lo, hi int, line sourceLine) (Value, *Error) {
	if lo >= hi {
		return Value{}, newError(UnsupportedSyntax, listEnd(tokens), "missing expression")
	}
	group := tokens[lo:hi]
	if keyed := keyedLiteralName(group); keyed != "" {
		keys, idents, identErr := keyedLiteralScan(group)
		if identErr != nil {
			return Value{}, identErr
		}
		return Value{
			Text:   line.raw[tokens[lo].start-line.offset : tokens[hi-1].end-line.offset],
			Idents: idents,
			Span:   Span{Start: tokens[lo].start, End: tokens[hi-1].end},
			Keyed: &KeyedLiteral{
				Name:   keyed,
				Fields: keys,
				Span:   Span{Start: tokens[lo].start, End: tokens[hi-1].end},
			},
		}, nil
	}
	navigation, idents, recognized, navErr := navigationMetadata(group)
	if navErr != nil {
		return Value{}, navErr
	}
	if !recognized {
		if validationErr := validateNavigationTokens(group); validationErr != nil {
			return Value{}, validationErr
		}
		var identErr *Error
		idents, identErr = valueIdents(group)
		if identErr != nil {
			return Value{}, identErr
		}
	}
	return Value{
		Text:       line.raw[tokens[lo].start-line.offset : tokens[hi-1].end-line.offset],
		Idents:     idents,
		Navigation: navigation,
		Span:       Span{Start: tokens[lo].start, End: tokens[hi-1].end},
	}, nil
}

// keyedLiteralName reports the type name of a keyed construction group
// `N{ … }` (RFC-014 §6.3); empty when the group has another shape.
func keyedLiteralName(group []token) string {
	if len(group) < 3 || group[0].kind != tokenIdent || reservedWords[group[0].text] || !isPunct(group[1], "{") || !isPunct(group[len(group)-1], "}") {
		return ""
	}
	return group[0].text
}

// keyedLiteralIdents collects the binding reads of a keyed construction:
// the type name and the field keys are not reads (they name declared
// structure, not values); only the field-value expressions count, scanned
// recursively so nested literals hide their keys too.
// keyedLiteralScan walks a keyed construction group and returns the
// ordered field keys plus the binding reads of the field-value expressions
// (keys and the type name are never reads).
func keyedLiteralScan(group []token) ([]string, []string, *Error) {
	var keys []string
	var idents []string
	seen := map[string]bool{}
	i := 2 // past the type name and the opening brace
	for i < len(group)-1 {
		t := group[i]
		switch {
		case t.kind == tokenIdent && !reservedWords[t.text] && i+1 < len(group)-1 && isPunct(group[i+1], ":"):
			// The key names a declared field, not a value: counted never,
			// duplicated never (RFC-014 §6.3 - exactly once).
			if seen[t.text] {
				return nil, nil, newError(UnsupportedSyntax, t.start, fmt.Sprintf("duplicate field key %q", t.text))
			}
			seen[t.text] = true
			keys = append(keys, t.text)
			i += 2 // past the key and the colon
			start, depth := i, 0
			for i < len(group)-1 && (depth > 0 || !isPunct(group[i], ",")) {
				if isPunct(group[i], "{") || isPunct(group[i], "(") || isPunct(group[i], "[") {
					depth++
				} else if isPunct(group[i], "}") || isPunct(group[i], ")") || isPunct(group[i], "]") {
					depth--
				}
				i++
			}
			segment := group[start:i]
			if keyedLiteralName(segment) != "" {
				_, nested, err := keyedLiteralScan(segment)
				if err != nil {
					return nil, nil, err
				}
				idents = append(idents, nested...)
			} else {
				segmentIdents, err := valueIdents(segment)
				if err != nil {
					return nil, nil, err
				}
				idents = append(idents, segmentIdents...)
			}
		case isPunct(t, "{"):
			depth := 1
			for i < len(group)-1 && depth > 0 {
				i++
				if isPunct(group[i], "{") {
					depth++
				} else if isPunct(group[i], "}") {
					depth--
				}
			}
		case isPunct(t, ":"):
			// A stray colon (no key) is skipped conservatively - its value
			// identifiers are not collected.
			i++
		default:
			i++
		}
	}
	return keys, idents, nil
}

// parseRHS parses the value region of a declaration or assignment RHS
// (story 24): a multiline keyed construction when the region is exactly
// `N{`, otherwise the ordinary single-line value list.
func (lp *lineParser) parseRHS(tokens []token, start int, line sourceLine) ([]Value, error) {
	// Story 32 (RFC-006 §6.4.2): a value-producing switch as the whole
	// RHS.
	if tokens[start].kind == tokenIdent && tokens[start].text == "switch" {
		value, verr := lp.parseSwitchValue(tokens, start, line)
		if verr != nil {
			return nil, verr
		}
		return []Value{value}, nil
	}
	value, handled, kerr := lp.parseMultilineKeyed(tokens, start, line)
	if kerr != nil {
		return nil, kerr
	}
	if handled {
		return []Value{value}, nil
	}
	return parseValueList(tokens, start, line)
}

// parseMultilineKeyed parses the multiline keyed construction `N{` … `}`
// (story 24, RFC-014 §6.4): the header is exactly `N{` at the end of the
// line, each entry line is `key: value,` (the trailing comma is MUST), and
// a bare `}` line closes the literal. Blank and `//` lines are skipped but
// stay in the verbatim text. Keys, reads, duplicates and nested
// single-line literals reuse the single-line scan over the collected
// token group; entry lines never reach parseValueList, so their commas
// cannot split the value list. Reports handled=false when the region is
// not exactly `N{`.
func (lp *lineParser) parseMultilineKeyed(tokens []token, start int, line sourceLine) (Value, bool, error) {
	if len(tokens)-start != 2 || tokens[start].kind != tokenIdent || reservedWords[tokens[start].text] || !isPunct(tokens[start+1], "{") {
		return Value{}, false, nil
	}
	group := []token{tokens[start], tokens[start+1]}
	var text strings.Builder
	text.WriteString(line.raw[tokens[start].start-line.offset:])
	lp.pos++
	for {
		if lp.pos >= len(lp.lines) {
			return Value{}, true, newError(UnsupportedSyntax, lp.lines[len(lp.lines)-1].offset, "missing closing }")
		}
		cur := lp.lines[lp.pos]
		if cur.text == "" || strings.HasPrefix(cur.text, "//") {
			text.WriteString(cur.raw)
			lp.pos++
			continue
		}
		if cur.text == "}" {
			closerTokens, terr := tokenize(cur.raw, cur.offset)
			if terr != nil {
				return Value{}, true, terr
			}
			text.WriteString(cur.raw[:closerTokens[0].end-cur.offset])
			group = append(group, closerTokens[0])
			lp.pos++
			value, kerr := keyedMultilineValue(group, text.String())
			return value, true, kerr
		}
		entryTokens, terr := tokenize(cur.raw, cur.offset)
		if terr != nil {
			return Value{}, true, terr
		}
		if kerr := validateKeyedEntry(entryTokens); kerr != nil {
			return Value{}, true, kerr
		}
		group = append(group, entryTokens...)
		text.WriteString(cur.raw)
		lp.pos++
	}
}

// validateKeyedEntry checks one multiline entry line `key: value,`
// (story 24, RFC-014 §6.4): the trailing comma is MUST, the value is a
// single-line expression - one top-level comma would be a second entry,
// and an unbalanced value is a nested multiline literal; both are
// explicit follow-up rejects.
func validateKeyedEntry(tokens []token) error {
	if len(tokens) < 4 || tokens[0].kind != tokenIdent || !isPunct(tokens[1], ":") {
		return newError(UnsupportedSyntax, tokens[0].start, "multiline literal entry must be `field: value,`")
	}
	if !isPunct(tokens[len(tokens)-1], ",") {
		return newError(UnsupportedSyntax, tokens[len(tokens)-1].start, "multiline literal entry must end with a comma")
	}
	depth := 0
	for _, t := range tokens[2 : len(tokens)-1] {
		switch {
		case isPunct(t, "{") || isPunct(t, "(") || isPunct(t, "["):
			depth++
		case isPunct(t, "}") || isPunct(t, ")") || isPunct(t, "]"):
			depth--
			if depth < 0 {
				return newError(UnsupportedSyntax, t.start, "unexpected closing bracket in entry value")
			}
		case depth == 0 && isPunct(t, ","):
			return newError(UnsupportedSyntax, t.start, "multiline literal entry must be one `field: value,` per line")
		case depth == 0 && (isAssign(t) || isPunct(t, "==")):
			return newError(UnsupportedSyntax, t.start, "unexpected = in entry value")
		}
	}
	if depth != 0 {
		return newError(UnsupportedSyntax, tokens[len(tokens)-2].start, "nested multiline literal is not supported in this slice")
	}
	return nil
}

// keyedMultilineValue builds the Value from the collected `N { entries }`
// token group: the shape, keys, reads, the duplicate-key check and nested
// single-line literals reuse the single-line scan verbatim.
func keyedMultilineValue(group []token, text string) (Value, error) {
	name := keyedLiteralName(group)
	if name == "" {
		return Value{}, newError(UnsupportedSyntax, listEnd(group), "invalid multiline keyed literal")
	}
	keys, idents, serr := keyedLiteralScan(group)
	if serr != nil {
		return Value{}, serr
	}
	return Value{
		Text:   text,
		Idents: idents,
		Keyed: &KeyedLiteral{
			Name:   name,
			Fields: keys,
			Span:   Span{Start: group[0].start, End: listEnd(group)},
		},
	}, nil
}

// isElseIfHeader reports a `} else if` continuation line that closes the
// current branch and opens a nested else-if statement.
func (lp *lineParser) isElseIfHeader(line sourceLine) bool {
	tokens, terr := tokenize(line.raw, line.offset)
	if terr != nil {
		return false
	}
	return len(tokens) >= 3 &&
		tokens[0].kind == tokenPunct && tokens[0].text == "}" &&
		tokens[1].kind == tokenIdent && tokens[1].text == "else" &&
		tokens[2].kind == tokenIdent && tokens[2].text == "if"
}
