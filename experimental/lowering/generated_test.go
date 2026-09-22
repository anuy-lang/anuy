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

// boundaryTestSource is the handwritten consumer-side test of the fixture
// module: it executes the generated boundary - valid calls reach the
// native body, invalid ones panic with the canonical carrier before the
// body runs (story 42, RFC-009 §6.8.3–6.8.7, §6.8.11–6.8.12). Recovery
// goes through errors.As on the carrier type, never by matching text
// (§6.8.12).
const boundaryTestSource = `package fixture

import (
	"errors"
	"strings"
	"testing"

	"github.com/anuy-lang/anuy/experimental/anuyabi"
)

type strReader struct{}

func (strReader) read() int { return 0 }

func TestForeignEntryBoundary(t *testing.T) {
	usr := User{id: 1}
	UseUser(&usr)
	if usr.id != 2 {
		t.Fatalf("valid call did not reach the native body: id = %d", usr.id)
	}
	inv := recoverBoundary(func() { UseUser(nil) })
	if inv == nil {
		t.Fatal("nil pointer passed the boundary")
	}
	if !strings.HasPrefix(inv.Error(), "anuy: invalid foreign value: ") {
		t.Fatalf("carrier message %q misses the stable prefix", inv.Error())
	}
	// With a nil input a reached body would die on a nil dereference, not
	// on the carrier - the typed recovery above proves the wrapper
	// panicked before the native implementation (§6.8.3).

	UseColor(Colorred)
	inv = recoverBoundary(func() { UseColor(Color(9)) })
	if inv == nil {
		t.Fatal("out-of-range enum discriminant passed the boundary (§6.8.6)")
	}

	UseColorOpt(anuyabi.Some(Colorred))
	inv = recoverBoundary(func() {
		UseColorOpt(anuyabi.Nullable[Color]{Value: Color(9), Present: true})
	})
	if inv == nil {
		t.Fatal("present invalid payload passed the boundary (§6.8.7)")
	}

	UseReader(strReader{})
	inv = recoverBoundary(func() {
		var p *strReader
		UseReader(p)
	})
	if inv == nil {
		t.Fatal("typed-nil dynamic value passed the boundary (§6.8.11)")
	}
}

func recoverBoundary(fn func()) *anuyabi.InvalidForeignValue {
	defer func() {
		_ = recover()
	}()
	var carrier *anuyabi.InvalidForeignValue
	func() {
		defer func() {
			err, ok := recover().(error)
			if !ok || !errors.As(err, &carrier) {
				carrier = nil
			}
		}()
		fn()
	}()
	return carrier
}
`

func TestLoweredForeignEntryBoundaryRuns(t *testing.T) {
	// Story 42: the compile-plus-run pin - the lowered fixture module
	// executes its own boundary test against the real anuyabi package.
	source, err := Lower(string(mustRead(t, filepath.Join("..", "fixtures", "lowering-core", "accept", "foreign-entry.anuy"))))
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
	if err := os.WriteFile(filepath.Join(dir, "boundary_test.go"), []byte(boundaryTestSource), 0o600); err != nil {
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
		t.Fatalf("boundary test failed: %v\n%s", err, out)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
