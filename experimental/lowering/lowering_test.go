package lowering

import "testing"

func TestLowerVarAssignmentToGo(t *testing.T) {
	got, err := Lower("var x\nx = 1\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\nfunc Run() {\n\tvar x any\n\tx = 1\n\t_ = x\n}\n"
	if got != want {
		t.Fatalf("Lower() = %q, want %q", got, want)
	}
}
