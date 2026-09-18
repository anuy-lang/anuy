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

func TestLoweredNullableGoCompiles(t *testing.T) {
	// Story 03-03 (RFC-009 §6.1.5 two-contracts sketch): generated Go with
	// tagged carriers depends on the anuyabi support package - the fixture
	// module requires the anuy module via a local replace, the way a real
	// consumer would require the released package (RFC-010). Only builtin-
	// base types keep the fixture self-contained; every binding is read,
	// so Go's declared-and-not-used does not fire.
	source, err := Lower("var x int? = 42\nvar s string?\ns = \"a\"\nvar n int? = x\ns\nn\n")
	if err != nil {
		t.Fatal(err)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	gomod := "module fixture\n\ngo 1.24\n\nrequire github.com/anuy-lang/anuy v0.0.0\n\nreplace github.com/anuy-lang/anuy => " + root + "\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go mod tidy failed: %v\n%s", err, out)
	}
	cmd = exec.Command("go", "test", ".")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated Go failed: %v\n%s", err, out)
	}
}
