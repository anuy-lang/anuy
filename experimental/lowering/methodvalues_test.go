package lowering

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Story 72 (RFC-019 §6.20-6.21/§6.25): method values and method
// expressions lower verbatim - Go's method value semantics save the
// receiver exactly once at value creation.
func TestLoweredMethodValueVerbatim(t *testing.T) {
	source, err := Lower("type Counter struct {\nn int\n}\nfunc (c Counter) Value() int {\nreturn c.n\n}\nvar c = Counter{n: 0}\nvar f = c.Value\nf\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, ":= c.Value") {
		t.Fatalf("generated Go lost the method value:\n%s", source)
	}
}

func TestLoweredMethodExpressionsVerbatim(t *testing.T) {
	source, err := Lower("type Counter struct {\nn int\n}\nfunc (c Counter) Value() int {\nreturn c.n\n}\nfunc (c *Counter) Inc() {\n}\nvar a = Counter.Value\nvar b = (*Counter).Inc\na\nb\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, ":= Counter.Value") || !strings.Contains(source, ":= (*Counter).Inc") {
		t.Fatalf("generated Go lost the method expressions:\n%s", source)
	}
}

// The generated Go with method values and both method expression forms
// compiles under the Go toolchain.
func TestLoweredMethodValuesGoCompiles(t *testing.T) {
	source, err := Lower("type Counter struct {\nn int\n}\nfunc (c Counter) Value() int {\nreturn c.n\n}\nfunc Apply(transform func() int) int {\nreturn transform()\n}\nvar c = Counter{n: 21}\nvar v = Apply(c.Value)\nv\n")
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
