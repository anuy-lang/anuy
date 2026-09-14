package lowering

import (
	"strings"
	"testing"
)

func TestLowerVarAssignmentToGo(t *testing.T) {
	got, err := Lower("var x int\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int\n\tx = 1\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerMultipleAssignmentToGo(t *testing.T) {
	got, err := Lower("var x int = 1\nvar y int = 2\nx, y = y, x\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\tvar y int = 2\n\tx, y = y, x\n\t_ = y\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerRejectsNullableType(t *testing.T) {
	if _, err := Lower("var u User?\n"); err == nil {
		t.Fatal("nullable type lowered")
	}
}

func TestLowerClosureLiteralToGo(t *testing.T) {
	got, err := Lower("var x int = 1\nvar increment = func() {\nx = x + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\tincrement := func() {\n\tx = x + 1\n}\n\t_ = increment\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerConditionLoopToGo(t *testing.T) {
	got, err := Lower("var ready bool = true\nvar x int\nfor ready {\nx = 1\n}\nx = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar ready bool = true\n\tvar x int\n\tfor ready {\n\tx = 1\n\t}\n\tx = 2\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerInfiniteLoopToGo(t *testing.T) {
	got, err := Lower("var x int\nfor {\nif x == 1 {\nbreak\n}\n}\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int\n\tfor {\n\tif x == 1 {\n\tbreak\n\t}\n\t}\n\tx = 1\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerIterationLoopToGo(t *testing.T) {
	got, err := Lower("var total int\nfor user in users {\ntotal = total + 1\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar total int\n\tfor _, user := range users {\n\ttotal = total + 1\n\t}\n\t_ = total\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerBreakContinuePassthrough(t *testing.T) {
	got, err := Lower("var i int = 0\nfor {\ni = i + 1\nif i == 3 {\nbreak\n}\nif i == 2 {\ncontinue\n}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar i int = 0\n\tfor {\n\ti = i + 1\n\tif i == 3 {\n\tbreak\n\t}\n\tif i == 2 {\n\tcontinue\n\t}\n\t}\n\t_ = i\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerRejectsUnsupportedRangeOperand(t *testing.T) {
	_, err := Lower("for x in getItems() {\n}\n")
	if err == nil {
		t.Fatal("unsupported range operand lowered")
	}
	if !strings.Contains(err.Error(), "unsupported range operand") {
		t.Fatalf("err = %v, want unsupported range operand error", err)
	}
}

func TestLowerBlockToGo(t *testing.T) {
	got, err := Lower("var x int = 1\n{\nx = 2\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x int = 1\n\t{\n\tx = 2\n\t}\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}

func TestLowerBlankDiscardToGo(t *testing.T) {
	got, err := Lower("var value, _ = f()\n_, y = 1, 2\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvalue, _ := f()\n\t_, y = 1, 2\n\t_ = y\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}
