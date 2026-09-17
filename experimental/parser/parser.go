// Package parser implements a deliberately restricted experimental grammar.
package parser

import (
	"fmt"
	"strings"

	"github.com/san-smith/anuy/experimental/semantic"
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
	Segments []NavigationSegment
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
	// Closure is non-nil for closure literals.
	Closure *Closure
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
	// Method is non-empty for a method declaration (story 08, owner
	// decision 2026-09-17, task-1-8-1-2): the receiver type name as written
	// (`User` in `func User.age() int`). The declared name is Names[0];
	// methods bind no scope name - resolution goes through the flat method
	// table.
	Method string
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
	// funcStack tracks declared result types of the enclosing
	// function/method bodies (story 08): `return expr` is valid only when
	// the top frame carries a result. Closure-literal frames push false,
	// so a `return expr` binds to the nearest declaration, never to an
	// outer one.
	funcStack []bool
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
	case tokens[0].kind == tokenIdent && tokens[0].text == "func" && len(tokens) >= 2 && tokens[1].kind == tokenIdent:
		// `func name(params) {` — the story 07 declaration form. `func (`
		// without a name stays a closure literal and is rejected below by
		// the call-statement path.
		return lp.parseFunctionDecl(tokens, line)
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
	case isCallStatementStart(tokens):
		return lp.parseCallStatement(tokens, line)
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
// { body }` and the story 08 extensions (owner decision 2026-09-17,
// task-1-8-1-2): the method form `func T.name(params) [ret] { body }`
// (RFC-002 §40) and the result type after the parameter list. The declared
// name binds a closure value; parameters and body reuse the closure grammar.
func (lp *lineParser) parseFunctionDecl(tokens []token, line sourceLine) (Statement, error) {
	name := tokens[1]
	if name.text == "_" || reservedWords[name.text] {
		return Statement{}, newError(UnsupportedSyntax, name.start, "invalid function name")
	}
	method := ""
	params := 2
	if len(tokens) >= 4 && isPunct(tokens[2], ".") {
		// `func T.name(params) …` - the method form; the receiver type stays
		// raw text (the flat method table resolves by name alone).
		m := tokens[3]
		if m.kind != tokenIdent || m.text == "_" || reservedWords[m.text] {
			return Statement{}, newError(UnsupportedSyntax, m.start, "invalid method name")
		}
		method, name, params = tokens[1].text, m, 4
	}
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
		Method:         method,
		HasResult:      cl.HasResult,
		ResultNullable: cl.ResultNullable,
		Closure:        &cl,
		Pure:           pure,
		Span:           Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseReturn parses the `return` statement. The bare form is an early
// exit of the enclosing function (story 07); `return expr` (story 08) is
// valid only inside a declaration with a declared result type - the
// funcStack frame of the nearest closure/function boundary decides.
func (lp *lineParser) parseReturn(tokens []token, line sourceLine) (Statement, error) {
	if len(tokens) == 1 {
		lp.pos++
		return Statement{
			Kind: Return,
			Span: Span{Start: line.offset, End: line.offset + len(line.text)},
		}, nil
	}
	if len(lp.funcStack) == 0 || !lp.funcStack[len(lp.funcStack)-1] {
		return Statement{}, newError(UnsupportedSyntax, tokens[1].start, "return takes no value")
	}
	values, verr := parseValueList(tokens, 1, line)
	if verr != nil {
		return Statement{}, verr
	}
	if len(values) != 1 {
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
	lp.pos++
	return Statement{
		Kind:   Call,
		Call:   &NavigationExpr{Receiver: tokens[0].text, Segments: segments},
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
		Call:   &NavigationExpr{Receiver: tokens[0].text, Segments: segments},
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

var reservedWords = map[string]bool{"var": true, "if": true, "else": true, "for": true, "break": true, "continue": true, "in": true, "nil": true, "true": true, "false": true, "return": true}

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
			return nil, newError(UnsupportedSyntax, base+i, fmt.Sprintf("unexpected character %q", c))
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
		values, verr := parseValueList(tokens, i+1, line)
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
		rest, verr := parseValueList(tokens, i+1, line)
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

// parseClosure parses `func(Params) {` plus its block. lp.pos is advanced
// past every consumed line.
func (lp *lineParser) parseClosure(tokens []token, start int, line sourceLine, allowResult bool) (Closure, *Error) {
	i, params, err := parseClosureParamGroups(tokens, start+1, line)
	if err != nil {
		return Closure{}, err
	}
	hasResult, resultNullable := false, false
	if i < len(tokens) && !(tokens[i].kind == tokenPunct && tokens[i].text == "{") {
		// An optional result type before the block (story 08 Q4-A, RFC-002
		// §40 spelling: `func f() User? {`). Types carry no braces in this
		// grammar, so the block opener terminates the type.
		if !allowResult {
			return Closure{}, newError(UnsupportedSyntax, tokens[i].start, "closure literal takes no result type")
		}
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
		i = j
	}
	if i >= len(tokens) || tokens[i].kind != tokenPunct || tokens[i].text != "{" {
		return Closure{}, newError(UnsupportedSyntax, listEnd(tokens), "closure requires a block")
	}
	cl := Closure{Params: params, HasResult: hasResult, ResultNullable: resultNullable, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	lp.pos++
	// A closure body is a function boundary: loop depth does not carry in,
	// and the result-type frame scopes `return expr` (story 08).
	savedDepth := lp.loopDepth
	lp.loopDepth = 0
	lp.funcStack = append(lp.funcStack, hasResult)
	body, berr := lp.parseBlock()
	lp.funcStack = lp.funcStack[:len(lp.funcStack)-1]
	lp.loopDepth = savedDepth
	if berr != nil {
		return Closure{}, berr.(*Error)
	}
	cl.Body = body
	return cl, nil
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
		return strings.ContainsRune("()[].?*+-", rune(t.text[0])) || t.text == "==" || t.text == "!="
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
			innerNavigation = &NavigationExpr{Receiver: inner[0].text}
			innerIdents = []string{inner[0].text}
		}
		return navigationSuffix(tokens, next, innerNavigation.Receiver, innerNavigation.Segments, innerIdents)
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
	return navigationSuffix(tokens, pos, tokens[0].text, nil, idents)
}

func navigationSuffix(tokens []token, pos int, receiver string, initial []NavigationSegment, idents []string) (*NavigationExpr, []string, bool, *Error) {
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
	return &NavigationExpr{Receiver: receiver, Segments: segments}, idents, true, nil
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
