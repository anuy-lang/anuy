package semantic

import (
	"math"
	"math/big"
	"strings"
)

// ConstKind classifies a constant value (RFC-018 §6.3): boolean, string
// or numeric - integers and exact rationals cover the untyped numeric
// tower (arbitrary precision, no machine width applies until a typed
// context converts the value).
type ConstKind uint8

const (
	ConstBool ConstKind = iota
	ConstString
	ConstInt
	ConstRat
)

// ConstValue is the exact value of a constant (RFC-018 §6.3/§6.29 v3).
type ConstValue struct {
	Kind ConstKind
	Bool bool
	Str  string
	Int  *big.Int
	Rat  *big.Rat
}

func ConstBoolOf(b bool) *ConstValue     { return &ConstValue{Kind: ConstBool, Bool: b} }
func ConstStringOf(s string) *ConstValue { return &ConstValue{Kind: ConstString, Str: s} }
func ConstIntOf(i *big.Int) *ConstValue  { return &ConstValue{Kind: ConstInt, Int: i} }
func ConstRatOf(r *big.Rat) *ConstValue  { return &ConstValue{Kind: ConstRat, Rat: r} }

// ConstEntry is one constant-environment record: the value plus the
// declared type for typed constants (§6.9/§8.4 - a typed expression
// keeps its type and every arithmetic step checks representability).
type ConstEntry struct {
	Value ConstValue
	Type  string
}

// ConstEvalKind classifies the evaluation failures of §8.17 v3:
// DivisionByZero (§8.5), Overflow (§8.4), Unresolved (the caller maps
// the name to its resolution diagnostics) and Unsupported (forms
// outside the grammar keep the go-stage backstop).
type ConstEvalKind uint8

const (
	ConstDivByZero ConstEvalKind = iota
	ConstOverflow
	ConstUnresolved
	ConstUnsupported
)

// ConstEvalError is one evaluation failure with the offending name for
// Unresolved.
type ConstEvalError struct {
	Kind ConstEvalKind
	Name string
}

func (e *ConstEvalError) Error() string {
	switch e.Kind {
	case ConstDivByZero:
		return "division by zero in constant expression"
	case ConstOverflow:
		return "constant expression is not representable in " + e.Name
	case ConstUnresolved:
		return "undefined constant " + e.Name
	default:
		return "unsupported constant expression"
	}
}

// constToken is one scanner token of the constant-expression grammar.
type constToken struct {
	kind byte // 'i' int, 'f' float, 's' string, 'n' name, 'o' operator
	text string
}

// scanConst tokens split a constant expression: numbers (integers and
// digits.digits floats), strings, names, operators and parentheses.
// Anything else is not part of the grammar - the caller sees
// Unsupported.
func scanConst(expr string) ([]constToken, bool) {
	var tokens []constToken
	i := 0
	for i < len(expr) {
		c := expr[i]
		switch {
		case c == ' ' || c == '\t':
			i++
		case c == '"':
			j := i + 1
			for j < len(expr) && expr[j] != '"' {
				j++
			}
			if j >= len(expr) {
				return nil, false
			}
			tokens = append(tokens, constToken{'s', expr[i : j+1]})
			i = j + 1
		case c >= '0' && c <= '9':
			j := i
			for j < len(expr) && expr[j] >= '0' && expr[j] <= '9' {
				j++
			}
			kind := byte('i')
			if j+1 < len(expr) && expr[j] == '.' && expr[j+1] >= '0' && expr[j+1] <= '9' {
				kind = 'f'
				j++
				for j < len(expr) && expr[j] >= '0' && expr[j] <= '9' {
					j++
				}
			}
			tokens = append(tokens, constToken{kind, expr[i:j]})
			i = j
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			j := i + 1
			for j < len(expr) && (expr[j] == '_' || expr[j] >= 'a' && expr[j] <= 'z' || expr[j] >= 'A' && expr[j] <= 'Z' || expr[j] >= '0' && expr[j] <= '9') {
				j++
			}
			tokens = append(tokens, constToken{'n', expr[i:j]})
			i = j
		case c == '!' && i+1 < len(expr) && expr[i+1] == '=':
			tokens = append(tokens, constToken{'o', "!="})
			i += 2
		case c == '<' || c == '>':
			if i+1 < len(expr) && expr[i+1] == '=' {
				tokens = append(tokens, constToken{'o', expr[i : i+2]})
				i += 2
				continue
			}
			tokens = append(tokens, constToken{'o', string(c)})
			i++
		case c == '=' && i+1 < len(expr) && expr[i+1] == '=':
			tokens = append(tokens, constToken{'o', "=="})
			i += 2
		case strings.ContainsRune("+-*/%()", rune(c)):
			tokens = append(tokens, constToken{'o', string(c)})
			i++
		default:
			return nil, false
		}
	}
	return tokens, true
}

// EvaluateConst evaluates a constant expression (RFC-018 §6.29 v3):
// exact untyped arithmetic (§6.3), comparisons (§6.15), string
// concatenation, parentheses, unary minus. lookup resolves constant
// names; Unresolved carries the missing name. A typed operand keeps the
// result typed - the untyped side converts contextually with
// representability checked at every step (§8.4). Forms outside the
// grammar report Unsupported (the caller keeps its go-stage backstop).
func EvaluateConst(expr string, lookup func(name string) (ConstEntry, bool)) (ConstValue, *ConstEvalError) {
	tokens, ok := scanConst(expr)
	if !ok {
		return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	p := &constParser{tokens: tokens, lookup: lookup}
	typed, err := p.comparison()
	if err != nil {
		return ConstValue{}, err
	}
	if p.pos != len(p.tokens) {
		return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	if typed.typ != "" && !FitToType(typed.value, typed.typ) {
		return ConstValue{}, &ConstEvalError{Kind: ConstOverflow, Name: typed.typ}
	}
	return typed.value, nil
}

type constParser struct {
	tokens []constToken
	pos    int
	lookup func(name string) (ConstEntry, bool)
}

func (p *constParser) peek() (constToken, bool) {
	if p.pos >= len(p.tokens) {
		return constToken{}, false
	}
	return p.tokens[p.pos], true
}

func (p *constParser) takeOperator(operators ...string) (string, bool) {
	token, ok := p.peek()
	if !ok || token.kind != 'o' {
		return "", false
	}
	for _, operator := range operators {
		if token.text == operator {
			p.pos++
			return token.text, true
		}
	}
	return "", false
}

// constTyped is one evaluated operand with its constant type context:
// a typed operand keeps the result typed (§8.4 - every step checks
// representability), an untyped one stays exact.
type constTyped struct {
	value ConstValue
	typ   string
}

// comparison := additive ((< | <= | > | >= | == | !=) additive)?
// Comparisons yield an untyped bool (§6.15).
func (p *constParser) comparison() (constTyped, *ConstEvalError) {
	left, err := p.additive()
	if err != nil {
		return constTyped{}, err
	}
	operator, ok := p.takeOperator("<", "<=", ">", ">=", "==", "!=")
	if !ok {
		return left, nil
	}
	right, err := p.additive()
	if err != nil {
		return constTyped{}, err
	}
	value, err := compareConsts(left.value, operator, right.value)
	if err != nil {
		return constTyped{}, err
	}
	return constTyped{value: value}, nil
}

// additive := multiplicative ((+ | -) multiplicative)*
func (p *constParser) additive() (constTyped, *ConstEvalError) {
	left, err := p.multiplicative()
	if err != nil {
		return constTyped{}, err
	}
	for {
		operator, ok := p.takeOperator("+", "-")
		if !ok {
			return left, nil
		}
		right, err := p.multiplicative()
		if err != nil {
			return constTyped{}, err
		}
		left, err = applyConstArithmetic(left, right, operator)
		if err != nil {
			return constTyped{}, err
		}
	}
}

// multiplicative := unary ((* | / | %) unary)*
func (p *constParser) multiplicative() (constTyped, *ConstEvalError) {
	left, err := p.unary()
	if err != nil {
		return constTyped{}, err
	}
	for {
		operator, ok := p.takeOperator("*", "/", "%")
		if !ok {
			return left, nil
		}
		right, err := p.unary()
		if err != nil {
			return constTyped{}, err
		}
		left, err = applyConstArithmetic(left, right, operator)
		if err != nil {
			return constTyped{}, err
		}
	}
}

// unary := - unary | primary
func (p *constParser) unary() (constTyped, *ConstEvalError) {
	if _, ok := p.takeOperator("-"); ok {
		value, err := p.unary()
		if err != nil {
			return constTyped{}, err
		}
		value.value, err = applyConstUnaryMinus(value.value)
		if err != nil {
			return constTyped{}, err
		}
		return p.fitTyped(value)
	}
	return p.primary()
}

func (p *constParser) primary() (constTyped, *ConstEvalError) {
	token, ok := p.peek()
	if !ok {
		return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	switch token.kind {
	case 'i':
		p.pos++
		n, ok := new(big.Int).SetString(token.text, 10)
		if !ok {
			return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
		}
		return constTyped{value: *ConstIntOf(n)}, nil
	case 'f':
		p.pos++
		r, ok := new(big.Rat).SetString(token.text)
		if !ok {
			return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
		}
		return constTyped{value: *ConstRatOf(r)}, nil
	case 's':
		p.pos++
		return constTyped{value: *ConstStringOf(strings.Trim(token.text, `"`))}, nil
	case 'n':
		p.pos++
		if token.text == "true" || token.text == "false" {
			// Predeclared boolean literals (§6.3) - language constants,
			// not environment entries.
			return constTyped{value: *ConstBoolOf(token.text == "true")}, nil
		}
		entry, found := p.lookup(token.text)
		if !found {
			return constTyped{}, &ConstEvalError{Kind: ConstUnresolved, Name: token.text}
		}
		return constTyped{value: entry.Value, typ: entry.Type}, nil
	case 'o':
		if token.text == "(" {
			p.pos++
			value, err := p.comparison()
			if err != nil {
				return constTyped{}, err
			}
			closing, ok := p.takeOperator(")")
			if !ok || closing != ")" {
				return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
			}
			return value, nil
		}
	}
	return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
}

// asNumeric returns the exact numeric view: an integer participates in
// rational arithmetic as an exact rational.
func asNumeric(value ConstValue) (*big.Int, *big.Rat, bool) {
	switch value.Kind {
	case ConstInt:
		return value.Int, nil, true
	case ConstRat:
		return nil, value.Rat, true
	}
	return nil, nil, false
}

// fitTyped applies the operand's typed context: the value must be
// representable in its constant type (§8.4 - typed constants never
// wrap).
func (p *constParser) fitTyped(value constTyped) (constTyped, *ConstEvalError) {
	if value.typ != "" && !FitToType(value.value, value.typ) {
		return constTyped{}, &ConstEvalError{Kind: ConstOverflow, Name: value.typ}
	}
	return value, nil
}

func applyConstUnaryMinus(value ConstValue) (ConstValue, *ConstEvalError) {
	switch value.Kind {
	case ConstInt:
		return *ConstIntOf(new(big.Int).Neg(value.Int)), nil
	case ConstRat:
		return *ConstRatOf(new(big.Rat).Neg(value.Rat)), nil
	}
	return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
}

// applyConstArithmetic computes one exact arithmetic step (§6.7/§6.3):
// integers and rationals mix as exact numbers, strings concatenate with
// `+`, remainder is integer-only. Division by zero errors (§8.5). A
// typed operand keeps the result typed - the untyped side converts
// contextually (representability) and the result is fitted (§8.4).
func applyConstArithmetic(left, right constTyped, operator string) (constTyped, *ConstEvalError) {
	if left.value.Kind == ConstString && right.value.Kind == ConstString && operator == "+" {
		if left.typ != "" || right.typ != "" {
			return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
		}
		return constTyped{value: *ConstStringOf(left.value.Str + right.value.Str)}, nil
	}
	leftInt, leftRat, leftNum := asNumeric(left.value)
	rightInt, rightRat, rightNum := asNumeric(right.value)
	if !leftNum || !rightNum {
		return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	if leftInt != nil && rightInt != nil {
		switch operator {
		case "+":
			return p_fit(left, right, *ConstIntOf(new(big.Int).Add(leftInt, rightInt)))
		case "-":
			return p_fit(left, right, *ConstIntOf(new(big.Int).Sub(leftInt, rightInt)))
		case "*":
			return p_fit(left, right, *ConstIntOf(new(big.Int).Mul(leftInt, rightInt)))
		case "/":
			if rightInt.Sign() == 0 {
				return constTyped{}, &ConstEvalError{Kind: ConstDivByZero}
			}
			if new(big.Int).Rem(leftInt, rightInt).Sign() == 0 {
				return p_fit(left, right, *ConstIntOf(new(big.Int).Quo(leftInt, rightInt)))
			}
			return p_fit(left, right, *ConstRatOf(new(big.Rat).SetFrac(leftInt, rightInt)))
		case "%":
			if rightInt.Sign() == 0 {
				return constTyped{}, &ConstEvalError{Kind: ConstDivByZero}
			}
			return p_fit(left, right, *ConstIntOf(new(big.Int).Rem(leftInt, rightInt)))
		}
		return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	leftRat = ratOf(leftInt, leftRat)
	rightRat = ratOf(rightInt, rightRat)
	switch operator {
	case "+":
		return p_fit(left, right, *ConstRatOf(new(big.Rat).Add(leftRat, rightRat)))
	case "-":
		return p_fit(left, right, *ConstRatOf(new(big.Rat).Sub(leftRat, rightRat)))
	case "*":
		return p_fit(left, right, *ConstRatOf(new(big.Rat).Mul(leftRat, rightRat)))
	case "/":
		if rightRat.Sign() == 0 {
			return constTyped{}, &ConstEvalError{Kind: ConstDivByZero}
		}
		result := new(big.Rat).Quo(leftRat, rightRat)
		if result.IsInt() {
			return p_fit(left, right, *ConstIntOf(new(big.Int).Set(result.Num())))
		}
		return p_fit(left, right, *ConstRatOf(result))
	case "%":
		return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	return constTyped{}, &ConstEvalError{Kind: ConstUnsupported}
}

// p_fit applies the typed context of either operand to the result: the
// untyped side converts contextually (fit check), the typed side names
// the result type and the result itself must represent (§8.4).
func p_fit(left, right constTyped, result ConstValue) (constTyped, *ConstEvalError) {
	typ := left.typ
	if typ == "" {
		typ = right.typ
	}
	if typ == "" {
		return constTyped{value: result}, nil
	}
	other := right
	otherTypeOwner := left
	if right.typ != "" {
		other, otherTypeOwner = left, right
	}
	if other.typ == "" && !FitToType(other.value, typ) {
		return constTyped{}, &ConstEvalError{Kind: ConstOverflow, Name: typ}
	}
	if !FitToType(result, otherTypeOwner.typ) {
		return constTyped{}, &ConstEvalError{Kind: ConstOverflow, Name: otherTypeOwner.typ}
	}
	return constTyped{value: result, typ: otherTypeOwner.typ}, nil
}

func ratOf(i *big.Int, r *big.Rat) *big.Rat {
	if r != nil {
		return r
	}
	return new(big.Rat).SetInt(i)
}

// compareConsts evaluates one comparison (§6.15): numeric ordering over
// the exact tower, lexicographic string ordering, boolean equality.
func compareConsts(left ConstValue, operator string, right ConstValue) (ConstValue, *ConstEvalError) {
	if left.Kind == ConstBool && right.Kind == ConstBool {
		switch operator {
		case "==":
			return *ConstBoolOf(left.Bool == right.Bool), nil
		case "!=":
			return *ConstBoolOf(left.Bool != right.Bool), nil
		}
		return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	if left.Kind == ConstString && right.Kind == ConstString {
		cmp := strings.Compare(left.Str, right.Str)
		return *ConstBoolOf(applyComparison(operator, cmp)), nil
	}
	leftInt, leftRat, ok := asNumeric(left)
	if !ok {
		return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	rightInt, rightRat, ok := asNumeric(right)
	if !ok {
		return ConstValue{}, &ConstEvalError{Kind: ConstUnsupported}
	}
	cmp := ratOf(leftInt, leftRat).Cmp(ratOf(rightInt, rightRat))
	return *ConstBoolOf(applyComparison(operator, cmp)), nil
}

func applyComparison(operator string, cmp int) bool {
	switch operator {
	case "<":
		return cmp < 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case ">=":
		return cmp >= 0
	case "==":
		return cmp == 0
	default:
		return cmp != 0
	}
}

// numericTypeRanges pins the Go basic integer widths (§6.4): the
// checker converts constants contextually against these ranges.
var numericTypeRanges = map[string]*big.Int{
	"int8":   big.NewInt(1 << 7),
	"int16":  big.NewInt(1 << 15),
	"int32":  big.NewInt(1 << 31),
	"int64":  new(big.Int).Lsh(big.NewInt(1), 63),
	"int":    new(big.Int).Lsh(big.NewInt(1), 63),
	"uint8":  big.NewInt(1 << 8),
	"uint16": big.NewInt(1 << 16),
	"uint32": big.NewInt(1 << 32),
	"uint64": new(big.Int).Lsh(big.NewInt(1), 64),
	"uint":   new(big.Int).Lsh(big.NewInt(1), 64),
	"byte":   big.NewInt(1 << 8),
	"rune":   big.NewInt(1 << 31),
}

// IsNumericBasic reports whether the name is a Go basic numeric type
// with a known representability rule (§6.4) - the typed-declaration fit
// check applies to it.
func IsNumericBasic(typeName string) bool {
	if _, ok := numericTypeRanges[typeName]; ok {
		return true
	}
	return typeName == "float32" || typeName == "float64"
}

// FitToType reports whether the constant value is representable in the
// named Go basic type (§6.4/§8.1): integer contexts require exact
// integers in range, float contexts round by magnitude, string and
// bool take their own kinds. Unknown type names fit nothing - the
// caller keeps its tolerance for non-basic types.
func FitToType(value ConstValue, typeName string) bool {
	bound, signed := numericBound(typeName)
	if bound != nil {
		return fitsInteger(value, bound, signed)
	}
	switch typeName {
	case "float32", "float64":
		return fitsFloat(value, typeName)
	case "string":
		return value.Kind == ConstString
	case "bool":
		return value.Kind == ConstBool
	}
	return false
}

func numericBound(typeName string) (*big.Int, bool) {
	bound, ok := numericTypeRanges[typeName]
	if !ok {
		return nil, false
	}
	return bound, typeName[0] != 'u' && typeName != "byte" && typeName != "uintptr"
}

func fitsInteger(value ConstValue, bound *big.Int, signed bool) bool {
	var n *big.Int
	switch value.Kind {
	case ConstInt:
		n = value.Int
	case ConstRat:
		if !value.Rat.IsInt() {
			return false // §8.11: a fractional constant truncates nowhere
		}
		n = new(big.Int).Set(value.Rat.Num())
	default:
		return false
	}
	if signed {
		limit := new(big.Int).Sub(bound, big.NewInt(1))
		return n.Cmp(new(big.Int).Neg(limit)) >= 0 && n.Cmp(limit) <= 0
	}
	return n.Sign() >= 0 && n.Cmp(bound) < 0
}

func fitsFloat(value ConstValue, typeName string) bool {
	var r *big.Rat
	switch value.Kind {
	case ConstInt:
		r = new(big.Rat).SetInt(value.Int)
	case ConstRat:
		r = value.Rat
	default:
		return false
	}
	if typeName == "float32" {
		f, exact := r.Float32()
		return !isInf32(f) || exact
	}
	f, exact := r.Float64()
	return f == f && f != inf64 && f != -inf64 || exact
}

var inf64 = math.Inf(1)

func isInf32(f float32) bool {
	return f > 3.4028235e38 || f < -3.4028235e38
}
