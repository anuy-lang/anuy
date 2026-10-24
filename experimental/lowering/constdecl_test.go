package lowering

import (
	"strings"
	"testing"
)

// Story 80 (RFC-018 §6.29 v2, RFC-024 §6.23 v4): a package constant
// lowers to an ordinary Go const declaration, verbatim expression, in
// canonical declaration order (§6.7 RFC-024).
func TestLowerPackageConstantVerbatim(t *testing.T) {
	got, err := Lower("const MaxRetries = 5\nfunc shout() {\nMaxRetries\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n//line source.anuy:1\nconst MaxRetries = 5\n//line source.anuy:2\nfunc shout() {\n//line source.anuy:3\n\t_ = MaxRetries\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// The typed form keeps the declared type verbatim.
func TestLowerTypedPackageConstant(t *testing.T) {
	got, err := Lower("const Max int = 5\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "const Max int = 5") {
		t.Fatalf("output misses the typed const:\n%s", got)
	}
}

// Constants render before the variables (reading order of the source).
func TestLowerConstantsPrecedeVariables(t *testing.T) {
	got, err := Lower("var v = 1\nconst C = 2\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(got, "const C = 2") > strings.Index(got, "var v = 1") {
		t.Fatalf("constants must precede variables:\n%s", got)
	}
}

// §6.19 RFC-024: the test variant carries the constants.
func TestLowerTestPackageCarriesConstants(t *testing.T) {
	got, err := LowerTestPackage([]File{{Path: "x_test.anuy", Source: "package demo_test\nconst MaxRetries = 5\nfunc TestX() {\nMaxRetries\n}\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "const MaxRetries = 5") {
		t.Fatalf("test variant misses the constant:\n%s", got)
	}
}
