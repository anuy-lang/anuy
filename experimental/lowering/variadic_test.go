package lowering

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Story 71 (RFC-019 §6.14/§6.25): variadics lower verbatim into native Go
// variadics - the declared signature keeps the `...` spelling, the spread
// call passes through, no wrappers.
func TestLoweredVariadicVerbatim(t *testing.T) {
	source, err := Lower("func Sum(values ...int) int {\nreturn 0\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "func Sum(values ...int) int {") {
		t.Fatalf("generated Go lost the variadic signature:\n%s", source)
	}
}

func TestLoweredSpreadVerbatim(t *testing.T) {
	source, err := Lower("func Sum(values ...int) int {\nreturn 0\n}\nvar v = Make()\nvar total = Sum(v...)\ntotal\nfunc Make() []int {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "Sum(v...)") {
		t.Fatalf("generated Go lost the spread call:\n%s", source)
	}
}

// The generated Go with variadic declarations, individual arguments and
// a spread call compiles under the Go toolchain.
func TestLoweredVariadicGoCompiles(t *testing.T) {
	source, err := Lower("func Sum(values ...int) int {\nvar total = 0\nfor v in values {\ntotal = total + v\n}\nreturn total\n}\nvar a = Sum(1, 2)\nvar b = Sum(a, 3)\nb\n")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "test", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go failed: %v\n%s", err, out)
	}
}
