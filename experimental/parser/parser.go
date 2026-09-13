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
	// Text is the raw source text of a non-closure expression. The
	// experimental grammar does not interpret it yet.
	Text string
	// Idents lists the identifiers referenced by a non-closure expression in
	// source order.
	Idents []string
	// Closure is non-nil for closure literals.
	Closure *Closure
}

// Closure is a `func(Params) { Body }` literal.
type Closure struct {
	Params []Param
	Body   []Statement
	Span   Span
}

// Param is a closure parameter; Type is the raw source text of its type.
type Param struct {
	Name string
	Type string
}

type Statement struct {
	Kind  Kind
	Names []string
	// Type is the declared type as raw source text, or "" for inferred
	// declarations.
	Type string
	// Values holds right-hand side expressions.
	Values []Value
	// Cond is the if condition as raw source text; CondIdents lists the
	// identifiers it references.
	Cond       string
	CondIdents []string
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
// conditions are kept as raw source text; the experimental grammar does not
// interpret them yet.
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
	var condIdents []string
	for _, t := range condition {
		if t.kind == tokenBlank {
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "blank identifier is not part of the confirmed grammar"}
		}
		if t.kind == tokenIdent && (t.text == "var" || t.text == "if" || t.text == "else") {
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected keyword %q in condition", t.text)}
		}
		if !isValueToken(t) {
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected token %q in condition", t.text)}
		}
		if t.kind == tokenIdent && !reservedWords[t.text] {
			condIdents = append(condIdents, t.text)
		}
	}
	statement := Statement{
		Kind:       If,
		Cond:       line.raw[condition[0].start-line.offset : condition[len(condition)-1].end-line.offset],
		CondIdents: condIdents,
		Span:       Span{Start: line.offset, End: line.offset + len(line.text)},
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

func isTypeToken(t token) bool {
	switch t.kind {
	case tokenInt:
		return true
	case tokenIdent:
		return !reservedWords[t.text]
	case tokenPunct:
		return strings.ContainsRune("*[]().?", rune(t.text[0]))
	default:
		return false
	}
}

func (lp *lineParser) parseVar(tokens []token, line sourceLine) (Statement, error) {
	names, i, terr := parseNameList(tokens, 1)
	if terr != nil {
		return Statement{}, terr
	}
	statement := Statement{Kind: Var, Names: names, Span: Span{Start: line.offset, End: line.offset + len(line.text)}}
	typeStart, typeEnd := -1, -1
	for i < len(tokens) && !isAssign(tokens[i]) {
		if !isTypeToken(tokens[i]) {
			return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[i].start, Message: "invalid type expression"}
		}
		if typeStart < 0 {
			typeStart = i
		}
		typeEnd = i
		i++
	}
	if len(names) > 1 && typeStart >= 0 {
		return Statement{}, &Error{Category: TypedMultipleDeclaration, Offset: tokens[typeStart].start, Message: "typed multiple declaration is excluded from the confirmed grammar"}
	}
	if typeStart >= 0 {
		statement.Type = line.raw[tokens[typeStart].start-line.offset : tokens[typeEnd].end-line.offset]
	} else if i >= len(tokens) {
		return Statement{}, &Error{Category: BareDeclaration, Offset: tokens[0].start, Message: "declaration requires a type or an initializer"}
	}
	if i < len(tokens) {
		values, err := lp.parseValues(tokens, i+1, line, names)
		if err != nil {
			return Statement{}, err
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
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "assignment requires ="}
	}
	values, err := lp.parseValues(tokens, i+1, line, names)
	if err != nil {
		return Statement{}, err
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
	lp.pos++
	return Statement{
		Kind:   Assign,
		Names:  names,
		Values: values,
		Span:   Span{Start: line.offset, End: line.offset + len(line.text)},
	}, nil
}

// parseValues parses the right-hand side after `=`. A closure literal must be
// the only value of a single-binding statement (RFC-003 §41, §80); all other
// values are single-line top-level comma-separated expressions.
func (lp *lineParser) parseValues(tokens []token, start int, line sourceLine, names []string) ([]Value, error) {
	if start >= len(tokens) {
		return nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
	}
	if isClosureStart(tokens, start) {
		if len(names) != 1 {
			return nil, &Error{Category: UnsupportedSyntax, Offset: tokens[start].start, Message: "closure initializer requires a single binding"}
		}
		cl, cerr := lp.parseClosure(tokens, start, line)
		if cerr != nil {
			return nil, cerr
		}
		return []Value{{Closure: &cl}}, nil
	}
	return parseValueList(tokens, start, line)
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
		for _, t := range g[1:] {
			if !isTypeToken(t) {
				return Closure{}, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "invalid parameter type"}
			}
		}
		params = append(params, Param{
			Name: g[0].text,
			Type: line.raw[g[1].start-line.offset : g[len(g)-1].end-line.offset],
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

// parseValueList splits the remaining tokens into top-level comma-separated
// expressions and keeps each expression as raw source text.
func parseValueList(tokens []token, start int, line sourceLine) ([]Value, error) {
	if start >= len(tokens) {
		return nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
	}
	var values []Value
	groupStart := start
	depth := 0
	i := start
	for ; i < len(tokens); i++ {
		t := tokens[i]
		switch {
		case t.kind == tokenPunct && (t.text == "(" || t.text == "["):
			depth++
		case t.kind == tokenPunct && (t.text == ")" || t.text == "]"):
			depth--
			if depth < 0 {
				return nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unbalanced brackets"}
			}
		case t.kind == tokenPunct && t.text == "," && depth == 0:
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
	if depth != 0 {
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
	var idents []string
	for _, t := range tokens[lo:hi] {
		if t.kind == tokenIdent {
			if t.text == "var" || t.text == "if" || t.text == "else" {
				return Value{}, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected keyword %q in expression", t.text)}
			}
			if !reservedWords[t.text] {
				idents = append(idents, t.text)
			}
		}
	}
	return Value{
		Text:   line.raw[tokens[lo].start-line.offset : tokens[hi-1].end-line.offset],
		Idents: idents,
	}, nil
}
