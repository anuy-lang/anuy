// Package parser implements a deliberately restricted experimental grammar.
package parser

import (
	"fmt"
	"strings"
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
)

// Error reports a parse-level reject with its category and byte offset.
type Error struct {
	Category ErrorCategory
	Offset   int
	Message  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("experimental parser: %s at %d", e.Message, e.Offset)
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
	// Cond is the if condition as raw source text; CondIdents lists the
	// binding identifiers it references. CondNavigation is non-nil for a
	// recognized selector chain.
	Cond           string
	CondIdents     []string
	CondNavigation *NavigationExpr
	// Body and Else hold the branch statements of an if statement.
	Body []Statement
	Else []Statement
	Span Span
}

type Program struct{ Statements []Statement }

// Parse accepts typed declarations, single and multiple assignment, closure
// literals in single-value initializers, if/else statements with blocks and
// bare identifier reads. Statements are line oriented; blocks open with `{`
// at the end of a header line and close with a `}` line. Expressions and
// conditions retain raw source text; recognized navigation chains additionally
// receive restricted structural validation and metadata.
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
}

func (lp *lineParser) parseStatements() ([]Statement, error) {
	var out []Statement
	for lp.pos < len(lp.lines) {
		line := lp.lines[lp.pos]
		if line.text == "" {
			lp.pos++
			continue
		}
		if line.text == "}" {
			return nil, &Error{Category: UnsupportedSyntax, Offset: line.offset, Message: "unexpected }"}
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
			return nil, &Error{Category: UnsupportedSyntax, Offset: lp.lines[len(lp.lines)-1].offset, Message: "missing closing }"}
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
		if lp.isElseHeader(line) {
			return out, nil
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, err
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
	case tokens[0].kind == tokenIdent && tokens[0].text == "else":
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[0].start, Message: "unexpected else"}
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

// parseIf parses `if <expression> {` plus its block and an optional
// `} else {` block. The condition expression is kept as raw source text.
func (lp *lineParser) parseIf(tokens []token, line sourceLine) (Statement, error) {
	last := tokens[len(tokens)-1]
	if len(tokens) < 3 || last.kind != tokenPunct || last.text != "{" {
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[0].start, Message: "if requires a block"}
	}
	condition := tokens[1 : len(tokens)-1]
	if condition[0].kind == tokenIdent && condition[0].text == "func" {
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: condition[0].start, Message: "closure literal is not allowed in a condition"}
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
	if lp.pos < len(lp.lines) && lp.isElseHeader(lp.lines[lp.pos]) {
		lp.pos++
		elseBody, err := lp.parseBlock()
		if err != nil {
			return Statement{}, err
		}
		statement.Else = elseBody
	}
	return statement, nil
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

var reservedWords = map[string]bool{"var": true, "if": true, "else": true, "nil": true, "true": true, "false": true}

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
				return nil, &Error{Category: UnsupportedSyntax, Offset: base + i, Message: "unterminated string literal"}
			}
			j++
			tokens = append(tokens, token{kind: tokenString, text: line[i:j], start: base + i, end: base + j})
			i = j
		case c == ':':
			if i+1 < len(line) && line[i+1] == '=' {
				return nil, &Error{Category: ShortDeclaration, Offset: base + i, Message: ":= is a syntax error"}
			}
			return nil, &Error{Category: UnsupportedSyntax, Offset: base + i, Message: fmt.Sprintf("unexpected character %q", c)}
		case c == '!':
			if i+1 < len(line) && line[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenPunct, text: "!=", start: base + i, end: base + i + 2})
				i += 2
				continue
			}
			return nil, &Error{Category: UnsupportedSyntax, Offset: base + i, Message: "force unwrap is not part of the language"}
		case strings.ContainsRune("=,.?()[]*+-{}", rune(c)):
			if c == '=' && i+1 < len(line) && line[i+1] == '=' {
				tokens = append(tokens, token{kind: tokenPunct, text: "==", start: base + i, end: base + i + 2})
				i += 2
				continue
			}
			tokens = append(tokens, token{kind: tokenPunct, text: string(c), start: base + i, end: base + i + 1})
			i++
		default:
			return nil, &Error{Category: UnsupportedSyntax, Offset: base + i, Message: fmt.Sprintf("unexpected character %q", c)}
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
			return nil, 0, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "expected identifier"}
		}
		if tokens[i].kind != tokenIdent {
			return nil, 0, &Error{Category: UnsupportedSyntax, Offset: tokens[i].start, Message: "expected identifier"}
		}
		if tokens[i].text == "_" {
			return nil, 0, &Error{Category: UnsupportedSyntax, Offset: tokens[i].start, Message: "blank identifier is not part of the confirmed grammar"}
		}
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
	return &Error{Category: UnsupportedSyntax, Offset: offset, Message: message}
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
		return Statement{}, &Error{Category: TypedMultipleDeclaration, Offset: tokens[typeStart].start, Message: "typed multiple declaration is excluded from the confirmed grammar"}
	}
	if typeStart < i {
		typeExpr, typeErr := parseType(tokens[typeStart:i])
		if typeErr != nil {
			return Statement{}, typeErr
		}
		statement.Type = line.raw[tokens[typeStart].start-line.offset : tokens[i-1].end-line.offset]
		statement.TypeExpr = typeExpr
	} else if i >= len(tokens) {
		return Statement{}, &Error{Category: BareDeclaration, Offset: tokens[0].start, Message: "declaration requires a type or an initializer"}
	}
	if i < len(tokens) {
		if isClosureStart(tokens, i+1) {
			if len(names) != 1 {
				return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[i+1].start, Message: "closure initializer requires a single binding"}
			}
			cl, cerr := lp.parseClosure(tokens, i+1, line)
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
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: offset, Message: "safe navigation is not an assignment target"}
		}
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "assignment requires ="}
	}
	var values []Value
	if isClosureStart(tokens, i+1) {
		if len(names) != 1 {
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[i+1].start, Message: "closure initializer requires a single binding"}
		}
		cl, cerr := lp.parseClosure(tokens, i+1, line)
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
		return Statement{}, &Error{Category: ArityMismatch, Offset: tokens[0].start, Message: "assignment arity mismatch"}
	}
	seen := make(map[string]bool, len(names))
	for _, name := range names {
		if seen[name] {
			return Statement{}, &Error{Category: DuplicateAssignmentTarget, Offset: tokens[0].start, Message: fmt.Sprintf("duplicate assignment target %q", name)}
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
func (lp *lineParser) parseClosure(tokens []token, start int, line sourceLine) (Closure, *Error) {
	i := start + 2
	depth := 1
	var groups [][]token
	cur := []token{}
	for ; ; i++ {
		if i >= len(tokens) {
			return Closure{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "unterminated closure parameter list"}
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
			return Closure{}, &Error{Category: UnsupportedSyntax, Offset: g[0].start, Message: "expected parameter name"}
		}
		if len(g) < 2 {
			return Closure{}, &Error{Category: UnsupportedSyntax, Offset: g[0].start, Message: "closure parameter requires a type"}
		}
		typeExpr, typeErr := parseType(g[1:])
		if typeErr != nil {
			return Closure{}, typeErr
		}
		params = append(params, Param{
			Name:     g[0].text,
			Type:     line.raw[g[1].start-line.offset : g[len(g)-1].end-line.offset],
			TypeExpr: typeExpr,
		})
	}
	if i >= len(tokens) || tokens[i].kind != tokenPunct || tokens[i].text != "{" {
		return Closure{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "closure requires a block"}
	}
	cl := Closure{Params: params, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	lp.pos++
	body, err := lp.parseBlock()
	if err != nil {
		return Closure{}, err.(*Error)
	}
	cl.Body = body
	return cl, nil
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
			return &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "blank identifier is not part of the confirmed grammar"}
		case t.kind == tokenIdent && (t.text == "var" || t.text == "if" || t.text == "else"):
			return &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected keyword %q in condition", t.text)}
		case isPunct(t, "(") || isPunct(t, "["):
			isCall := isPunct(t, "(") && i > 0 && isCallCallee(tokens[i-1])
			delimiters = append(delimiters, isCall)
		case isPunct(t, ")") || isPunct(t, "]"):
			if len(delimiters) == 0 {
				return &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unbalanced brackets"}
			}
			delimiters = delimiters[:len(delimiters)-1]
		case isPunct(t, ","):
			if len(delimiters) > 0 && delimiters[len(delimiters)-1] {
				continue
			}
			return &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected token %q in condition", t.text)}
		case !isValueToken(t):
			return &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected token %q in condition", t.text)}
		}
	}
	if len(delimiters) != 0 {
		return &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "unbalanced brackets"}
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
				return nil, nil, true, &Error{Category: UnsupportedSyntax, Offset: operator.start, Message: "ordinary selector cannot follow safe navigation"}
			}
			pos++
		default:
			return nil, nil, false, nil
		}
		if pos >= len(tokens) || tokens[pos].kind != tokenIdent || reservedWords[tokens[pos].text] {
			return nil, nil, true, &Error{Category: UnsupportedSyntax, Offset: operator.start, Message: "navigation operator requires a member name"}
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
				return 0, nil, &Error{Category: UnsupportedSyntax, Offset: tokens[i].start, Message: "unbalanced navigation call"}
			}
		}
	}
	return 0, nil, &Error{Category: UnsupportedSyntax, Offset: tokens[start].start, Message: "unterminated navigation call"}
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
				return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unbalanced navigation arguments"}
			}
		case isPunct(t, ",") && depth == 0:
			if i == groupStart {
				return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "missing navigation argument"}
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
		return nil, &Error{Category: UnsupportedSyntax, Offset: tokens[len(tokens)-1].end, Message: "unbalanced navigation arguments"}
	}
	if groupStart == len(tokens) {
		return nil, &Error{Category: UnsupportedSyntax, Offset: tokens[len(tokens)-1].end, Message: "missing navigation argument"}
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
		if t.text == "var" || t.text == "if" || t.text == "else" {
			return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected keyword %q in expression", t.text)}
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
		return nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
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
				return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unbalanced brackets"}
			}
			delimiters = delimiters[:len(delimiters)-1]
		case t.kind == tokenPunct && t.text == ",":
			if len(delimiters) > 0 {
				if delimiters[len(delimiters)-1] {
					continue
				}
				return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unexpected comma in expression"}
			}
			value, gerr := valueGroup(tokens, groupStart, i, line)
			if gerr != nil {
				return nil, gerr
			}
			values = append(values, value)
			groupStart = i + 1
		case t.kind == tokenPunct && (t.text == "=" || t.text == "=="):
			return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unexpected = in expression"}
		case t.kind == tokenBlank:
			return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "blank identifier is not part of the confirmed grammar"}
		case !isValueToken(t):
			return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected token %q in expression", t.text)}
		}
	}
	if len(delimiters) != 0 {
		return nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "unbalanced brackets"}
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
		return Value{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
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
