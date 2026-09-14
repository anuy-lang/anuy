package lowering

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestLoweredGoCompiles(t *testing.T) {
	source, err := Lower("var ready bool = true\nvar x int\nvar users []int\nvar total int\nfor ready {\nx = 1\n}\nfor u in users {\ntotal = total + u\n}\nfor {\nif x == 1 {\nbreak\n}\nx = 1\n}\nx\n")
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
