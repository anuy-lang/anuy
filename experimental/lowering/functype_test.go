package lowering

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Story 69 (RFC-019 §6.2): function types pass through to generated Go -
// the declared signature keeps the source spelling of the func-typed
// parameter, documentation names included.
func TestLoweredFuncTypeParamPassesThrough(t *testing.T) {
	source, err := Lower("func Apply(value int, transform func(int) int) int {\nreturn transform(value)\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(source, "func Apply(value int, transform func(int) int) int {") {
		t.Fatalf("generated Go lost the func-typed parameter:\n%s", source)
	}
}

// RFC-002 §6.8.8: the nullable whole-function type takes the native Go
// nil representation - no tagged carrier is introduced, and the nil
// comparison stays the plain Go form (RFC-009 §6.7.12).
func TestLoweredNullableFuncTypeStaysNativeNil(t *testing.T) {
	source, err := Lower("var cb (func(int) int)? = nil\nif cb != nil {\ncb(1)\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(source, "anuyabi.Nullable[") {
		t.Fatalf("nullable function type took the carrier:\n%s", source)
	}
	if !strings.Contains(source, "cb != nil") {
		t.Fatalf("nil comparison was not the plain Go form:\n%s", source)
	}
}

// The generated Go with func-typed bindings compiles under the Go
// toolchain: a declared func-typed parameter invoked through the
// parameter binding (the F-68-4 demo shape).
func TestLoweredFuncTypeGoCompiles(t *testing.T) {
	source, err := Lower("var transform func(int) int = func(v int) int {\nreturn v * 2\n}\nvar applied int = transform(21)\napplied\n")
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
