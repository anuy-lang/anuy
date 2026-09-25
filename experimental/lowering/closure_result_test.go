package lowering

import (
	"strings"
	"testing"
)

// Story 68 (RFC-008 §6.9.10 v4, §6.9.7 RFC-007): a result-carrying
// callback lowers to the validating wrapper with result propagation, and
// a plain closure literal renders its declared result type (closes
// F-55-1).

func TestLowerResultCarryingCallbackWrapper(t *testing.T) {
	// Story 55's scenario with a result: the wrapper validates the enum
	// parameter and returns the inner call's result.
	got, err := Lower("type Color enum {\nRed\nGreen\n}\nfunc run() {\ngoRegister(func(c Color) int {\nreturn 1\n})\n}\n")
	if err != nil {
		t.Fatalf("err = %v, want the result-carrying wrapper", err)
	}
	for _, want := range []string{
		"func(c Color) int {",
		`anuyabi.RequireEnum("c", uint32(c), 2)`,
		"return __anuy_cb(c)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("Lower() misses %q:\n%s", want, got)
		}
	}
}

func TestLowerClosureLiteralResultType(t *testing.T) {
	// A plain closure value renders its declared result type.
	got, err := Lower("var f = func(x int) int {\nreturn x\n}\nf(1)\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func(x int) int {") {
		t.Fatalf("Lower() misses the declared result type:\n%s", got)
	}
}
