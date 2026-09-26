package lowering

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Story 74 (RFC-019 §6.5 v6/§6.25): plain multi-results lower verbatim;
// the fallible shape keeps the story 35 strict ABI.
func TestLoweredPlainMultiResultVerbatim(t *testing.T) {
	source, err := Lower("func Split(v string) (string, string) {\nreturn v, v\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "func Split(v string) (string, string) {") {
		t.Fatalf("generated Go lost the plain multi-result signature:\n%s", source)
	}
}

func TestLoweredNullableFuncResultVerbatim(t *testing.T) {
	source, err := Lower("func Make() (func() int)? {\nreturn nil\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "func Make() func() int {") {
		t.Fatalf("generated Go lost the nullable whole-function result:\n%s", source)
	}
}

func TestLoweredResultListsGoCompiles(t *testing.T) {
	source, err := Lower("func Split(v string) (string, string) {\nreturn v, v\n}\nfunc Make() (func() int)? {\nreturn nil\n}\nvar a, b = Split(\"ab\")\nvar f = Make()\nvar r = a\nr\nb\nf\n")
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
