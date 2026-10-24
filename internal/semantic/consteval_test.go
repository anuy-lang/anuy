package semantic

import (
	"math/big"
	"strings"
	"testing"
)

func mustInt(t *testing.T, s string) *big.Int {
	t.Helper()
	n, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad int %q", s)
	}
	return n
}

func evalOK(t *testing.T, expr string, env map[string]ConstEntry) ConstValue {
	t.Helper()
	value, evalErr := EvaluateConst(expr, func(name string) (ConstEntry, bool) {
		entry, ok := env[name]
		return entry, ok
	})
	if evalErr != nil {
		t.Fatalf("EvaluateConst(%q) = %v, want a value", expr, evalErr)
	}
	return value
}

// §6.3: untyped integer arithmetic is exact - arbitrary precision, no
// machine width applies.
func TestEvaluateConstExactIntegers(t *testing.T) {
	value := evalOK(t, "1 + 2", nil)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "3")) != 0 {
		t.Fatalf("value = %+v, want 3", value)
	}
	value = evalOK(t, "2 * 21", nil)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "42")) != 0 {
		t.Fatalf("value = %+v, want 42", value)
	}
	value = evalOK(t, "-(1 + 1)", nil)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "-2")) != 0 {
		t.Fatalf("value = %+v, want -2", value)
	}
}

// §6.3/§6.7: untyped division is exact - a fractional result stays a
// rational, an even division normalizes to an integer.
func TestEvaluateConstExactDivision(t *testing.T) {
	value := evalOK(t, "1 / 2", nil)
	if value.Kind != ConstRat || value.Rat.Cmp(big.NewRat(1, 2)) != 0 {
		t.Fatalf("value = %+v, want the exact 1/2 rational", value)
	}
	value = evalOK(t, "10 / 2", nil)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "5")) != 0 {
		t.Fatalf("value = %+v, want 5", value)
	}
	value = evalOK(t, "10 % 3", nil)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "1")) != 0 {
		t.Fatalf("value = %+v, want 1", value)
	}
}

// §8.5: constant division by zero is a compile-time error.
func TestEvaluateConstDivisionByZero(t *testing.T) {
	_, evalErr := EvaluateConst("1 / 0", func(string) (ConstEntry, bool) { return ConstEntry{}, false })
	if evalErr == nil || evalErr.Kind != ConstDivByZero {
		t.Fatalf("err = %v, want ConstDivByZero", evalErr)
	}
	_, evalErr = EvaluateConst("10 % 0", func(string) (ConstEntry, bool) { return ConstEntry{}, false })
	if evalErr == nil || evalErr.Kind != ConstDivByZero {
		t.Fatalf("err = %v, want ConstDivByZero", evalErr)
	}
}

// §6.15: ordering comparisons over numbers and strings; equality over
// booleans.
func TestEvaluateConstComparisons(t *testing.T) {
	value := evalOK(t, "1 < 2", nil)
	if value.Kind != ConstBool || !value.Bool {
		t.Fatalf("value = %+v, want true", value)
	}
	value = evalOK(t, "2.5 > 2", nil)
	if value.Kind != ConstBool || !value.Bool {
		t.Fatalf("value = %+v, want true", value)
	}
	value = evalOK(t, "\"a\" < \"b\"", nil)
	if value.Kind != ConstBool || !value.Bool {
		t.Fatalf("value = %+v, want true", value)
	}
	value = evalOK(t, "1 == 2", nil)
	if value.Kind != ConstBool || value.Bool {
		t.Fatalf("value = %+v, want false", value)
	}
	value = evalOK(t, "true != false", nil)
	if value.Kind != ConstBool || !value.Bool {
		t.Fatalf("value = %+v, want true", value)
	}
}

// §6.7: string concatenation with `+`.
func TestEvaluateConstStringConcatenation(t *testing.T) {
	value := evalOK(t, `"a" + "b"`, nil)
	if value.Kind != ConstString || value.Str != "ab" {
		t.Fatalf("value = %+v, want ab", value)
	}
}

// §6.29 v3: constant references resolve through the environment -
// forward declarations included (the caller drives lazy evaluation).
func TestEvaluateConstReferences(t *testing.T) {
	env := map[string]ConstEntry{
		"A": {Value: *ConstIntOf(mustInt(t, "5"))},
	}
	value := evalOK(t, "A + 1", env)
	if value.Kind != ConstInt || value.Int.Cmp(mustInt(t, "6")) != 0 {
		t.Fatalf("value = %+v, want 6", value)
	}
	_, evalErr := EvaluateConst("Ghost + 1", func(string) (ConstEntry, bool) { return ConstEntry{}, false })
	if evalErr == nil || evalErr.Kind != ConstUnresolved || evalErr.Name != "Ghost" {
		t.Fatalf("err = %v, want ConstUnresolved Ghost", evalErr)
	}
}

// §6.9/§8.4: a typed operand keeps the result typed - the untyped side
// converts contextually and every step checks representability.
func TestEvaluateConstTypedOperandOverflow(t *testing.T) {
	env := map[string]ConstEntry{
		"X": {Value: *ConstIntOf(mustInt(t, "255")), Type: "uint8"},
	}
	_, evalErr := EvaluateConst("X + 1", func(name string) (ConstEntry, bool) {
		return env[name], env[name].Type != "" || name == "X"
	})
	if evalErr == nil || evalErr.Kind != ConstOverflow {
		t.Fatalf("err = %v, want ConstOverflow (256 is not a uint8)", evalErr)
	}
}

// §6.4/§8.1: representability of constants in Go basic types.
func TestFitToType(t *testing.T) {
	if !FitToType(*ConstIntOf(mustInt(t, "127")), "int8") {
		t.Fatal("127 must fit int8")
	}
	if FitToType(*ConstIntOf(mustInt(t, "128")), "int8") {
		t.Fatal("128 must not fit int8")
	}
	if !FitToType(*ConstIntOf(mustInt(t, "255")), "uint8") {
		t.Fatal("255 must fit uint8")
	}
	if FitToType(*ConstRatOf(big.NewRat(1, 2)), "int") {
		t.Fatal("the fractional 1/2 must not fit int (§8.11)")
	}
	if FitToType(*ConstIntOf(mustInt(t, "300")), "uint8") {
		t.Fatal("300 must not fit uint8")
	}
	if !FitToType(*ConstRatOf(big.NewRat(3, 2)), "float64") {
		t.Fatal("floats round - magnitude fits")
	}
	if FitToType(*ConstIntOf(mustInt(t, "1" + strings.Repeat("0", 400))), "float64") {
		t.Fatal("the magnitude overflows float64")
	}
	if FitToType(*ConstIntOf(mustInt(t, "5")), "string") {
		t.Fatal("an integer is not a string")
	}
	if !FitToType(*ConstStringOf("hi"), "string") {
		t.Fatal("a string fits string")
	}
}
