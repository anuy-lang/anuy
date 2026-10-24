package lowering

import (
	"strings"
	"testing"
)

// Story 78 (RFC-024 §6.21–6.22): a package variable lowers to an ordinary
// Go package-level variable with its explicit initializer, //line-anchored
// to its source statement (§6.7 canonical declaration order) - the Run
// body no longer carries it.
func TestLowerPackageVariableEmitsPackageLevel(t *testing.T) {
	got, err := Lower("var Greeting = \"hello\"\nfunc shout() {\nGreeting\n}\nshout()\n")
	if err != nil {
		t.Fatal(err)
	}
	want := "package fixture\n\n//line source.anuy:1\nvar Greeting = \"hello\"\n//line source.anuy:2\nfunc shout() {\n//line source.anuy:3\n\t_ = Greeting\n}\n\nfunc Run() {\n//line source.anuy:5\n\tshout()\n}\n"
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
}

// A typed package variable keeps its declared Go type.
func TestLowerTypedPackageVariableKeepsType(t *testing.T) {
	got, err := Lower("var cfg Config = load()\nfunc load() Config {\nreturn Config{}\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "var cfg Config = load()") {
		t.Fatalf("output misses the typed package var:\n%s", got)
	}
	if strings.Contains(got, "func Run() {\n//line source.anuy:1") {
		t.Fatalf("the package var leaked into the Run body:\n%s", got)
	}
}

// §6.6: a multi-variable group lowers verbatim as one Go declaration.
func TestLowerMultiNamePackageVariableGroup(t *testing.T) {
	got, err := Lower("func MakePair() (int, int) {\nreturn 1, 2\n}\nvar a, b = MakePair()\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "var a, b = MakePair()") {
		t.Fatalf("output misses the group declaration:\n%s", got)
	}
}

// §6.19: the test variant carries the package variables too.
func TestLowerTestPackageCarriesPackageVariables(t *testing.T) {
	got, err := LowerTestPackage([]File{{Path: "x_test.anuy", Source: "package demo_test\nvar Greeting = \"hi\"\nfunc TestX() {\nGreeting\n}\n"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "var Greeting = \"hi\"") {
		t.Fatalf("test variant misses the package var:\n%s", got)
	}
}

// A try declaration cannot initialize a package variable (§6.3/§6.11) -
// the lowering rejects deterministically instead of emitting invalid Go.
func TestLowerRejectsTryPackageVariable(t *testing.T) {
	if _, err := Lower("func F() (int, error?) {\nreturn 0, nil\n}\nvar v = try F()\n"); err == nil {
		t.Fatal("try package variable accepted")
	}
}

// The safe-tail guard branch is a body construct - a package variable
// with a safe-tail initializer rejects deterministically (§6.22 v2
// boundary).
func TestLowerRejectsSafeTailPackageVariable(t *testing.T) {
	if _, err := Lower("func Find() User? {\nreturn nil\n}\nvar u User? = Find()?.name\n"); err == nil {
		t.Fatal("safe-tail package variable accepted")
	}
}
