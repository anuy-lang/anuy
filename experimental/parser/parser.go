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

type Statement struct {
	Kind  Kind
	Names []string
	// Type is the declared type as raw source text, or "" for inferred
	// declarations.
	Type string
	// Values holds right-hand side expressions as raw source text. The
	// experimental grammar does not interpret them yet.
	Values []string
	// ValueIdents lists, for each entry of Values, the identifiers referenced
	// by the expression in source order.
	ValueIdents [][]string
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

// Parse accepts typed declarations, single and multiple assignment, if/else
// statements with blocks and bare identifier reads. Statements are line
// oriented; blocks open with `{` at the end of a header line and close with
// a `}` line. Values and conditions are kept as raw source text; the
// experimental grammar does not interpret them yet.
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
		if statement.Kind != If {
			lp.pos++
		}
	}
	return out, nil
}

// parseBlock parses statements until the closing } line, which it consumes.
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
			// The block closes here; the else header belongs to the if
			// statement that owns this block, which consumes it.
			return out, nil
		}
		statement, err := lp.parseStatement(line)
		if err != nil {
			return nil, err
		}
		out = append(out, statement)
		if statement.Kind != If {
			lp.pos++
		}
	}
}

func (lp *lineParser) parseStatement(line sourceLine) (Statement, error) {
	tokens, terr := tokenize(line.raw, line.offset)
	if terr != nil {
		return Statement{}, terr
	}
	var statement Statement
	var err *Error
	switch {
	case tokens[0].kind == tokenIdent && tokens[0].text == "var":
		statement, err = parseVar(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "if":
		return lp.parseIf(tokens, line)
	case tokens[0].kind == tokenIdent && tokens[0].text == "else":
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[0].start, Message: "unexpected else"}
	case len(tokens) == 1 && tokens[0].kind == tokenIdent:
		statement = Statement{Kind: Read, Names: []string{tokens[0].text}}
	default:
		statement, err = parseAssign(tokens, line)
	}
	if err != nil {
		return Statement{}, err
	}
	statement.Span = Span{Start: line.offset, End: line.offset + len(line.text)}
	return statement, nil
}

// parseIf parses `if <expression> {` plus its block and an optional
// `} else {` block. The condition expression is kept as raw source text.
func (lp *lineParser) parseIf(tokens []token, line sourceLine) (Statement, error) {
	last := tokens[len(tokens)-1]
	if len(tokens) < 3 || last.kind != tokenPunct || last.text != "{" {
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: tokens[0].start, Message: "if requires a block"}
	}
	condition := tokens[1 : len(tokens)-1]
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

func parseVar(tokens []token, line sourceLine) (Statement, *Error) {
	names, i, terr := parseNameList(tokens, 1)
	if terr != nil {
		return Statement{}, terr
	}
	statement := Statement{Kind: Var, Names: names}
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
		values, idents, verr := parseValueList(tokens, i+1, line)
		if verr != nil {
			return Statement{}, verr
		}
		statement.Values = values
		statement.ValueIdents = idents
	}
	return statement, nil
}

func parseAssign(tokens []token, line sourceLine) (Statement, *Error) {
	names, i, terr := parseNameList(tokens, 0)
	if terr != nil {
		return Statement{}, terr
	}
	if i >= len(tokens) || !isAssign(tokens[i]) {
		return Statement{}, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "assignment requires ="}
	}
	values, idents, verr := parseValueList(tokens, i+1, line)
	if verr != nil {
		return Statement{}, verr
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
	return Statement{Kind: Assign, Names: names, Values: values, ValueIdents: idents}, nil
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
func parseValueList(tokens []token, start int, line sourceLine) ([]string, [][]string, *Error) {
	if start >= len(tokens) {
		return nil, nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
	}
	var values []string
	var idents [][]string
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
				return nil, nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unbalanced brackets"}
			}
		case t.kind == tokenPunct && t.text == "," && depth == 0:
			value, group, gerr := valueGroup(tokens, groupStart, i, line)
			if gerr != nil {
				return nil, nil, gerr
			}
			values = append(values, value)
			idents = append(idents, group)
			groupStart = i + 1
		case t.kind == tokenPunct && (t.text == "=" || t.text == "=="):
			return nil, nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "unexpected = in expression"}
		case t.kind == tokenBlank:
			return nil, nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: "blank identifier is not part of the confirmed grammar"}
		case !isValueToken(t):
			return nil, nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected token %q in expression", t.text)}
		}
	}
	if depth != 0 {
		return nil, nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "unbalanced brackets"}
	}
	value, group, gerr := valueGroup(tokens, groupStart, i, line)
	if gerr != nil {
		return nil, nil, gerr
	}
	values = append(values, value)
	idents = append(idents, group)
	return values, idents, nil
}

func valueGroup(tokens []token, lo, hi int, line sourceLine) (string, []string, *Error) {
	if lo >= hi {
		return "", nil, &Error{Category: UnsupportedSyntax, Offset: listEnd(tokens), Message: "missing expression"}
	}
	raw := line.raw[tokens[lo].start-line.offset : tokens[hi-1].end-line.offset]
	var idents []string
	for _, t := range tokens[lo:hi] {
		if t.kind == tokenIdent {
			if t.text == "var" || t.text == "if" || t.text == "else" {
				return "", nil, &Error{Category: UnsupportedSyntax, Offset: t.start, Message: fmt.Sprintf("unexpected keyword %q in expression", t.text)}
			}
			if !reservedWords[t.text] {
				idents = append(idents, t.text)
			}
		}
	}
	return raw, idents, nil
}
