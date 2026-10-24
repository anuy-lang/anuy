package lowering

import (
	"strings"
	"testing"
)

// Story 79 (RFC-024 §6.21–6.22): the special init function lowers to an
// ordinary Go init function - it already runs before the program entry
// by Go's own package initialization.
func TestLowerInitFunctionVerbatim(t *testing.T) {
	got, err := Lower("var Greeting = \"unset\"\nfunc init() {\nGreeting = \"from init\"\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n//line source.anuy:1\nvar Greeting = \"unset\"\n//line source.anuy:2\nfunc init() {\n//line source.anuy:3\n\tGreeting = \"from init\"\n}\n\nfunc Run() {\n}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// §6.10/§6.7: multiple init functions keep the canonical declaration
// order - Go executes init functions in presentation order, which is
// the same order for one generated file.
func TestLowerMultipleInitFunctionsCanonicalOrder(t *testing.T) {
	got, err := Lower("var a = 1\nvar b = 2\nfunc init() {\na = 10\n}\nfunc init() {\nb = 20\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	first := strings.Index(got, "a = 10")
	second := strings.Index(got, "b = 20")
	if first < 0 || second < 0 || first > second {
		t.Fatalf("init order drifted:\n%s", got)
	}
	if strings.Count(got, "func init() {") != 2 {
		t.Fatalf("expected two init functions:\n%s", got)
	}
}

// §6.19: the test variant carries init functions - the test process
// performs the ordinary package initialization before tests.
func TestLowerTestPackageCarriesInit(t *testing.T) {
	got, err := LowerTestPackage([]File{{Path: "x_test.anuy", Source: "package demo_test\nvar Greeting = \"unset\"\nfunc init() {\nGreeting = \"from init\"\n}\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "func init() {") || !strings.Contains(got, "var Greeting = \"unset\"") {
		t.Fatalf("test variant misses the init machinery:\n%s", got)
	}
}
