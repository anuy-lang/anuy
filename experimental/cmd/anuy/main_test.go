package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTemp materializes a source file named test.anuy — the file name
// is part of the §6.12.1 human diagnostic format pins.
func writeTemp(t *testing.T, src string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.anuy")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// runCLI runs the command the way main would and returns the exit code
// with the captured streams.
func runCLI(args []string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

// §6.4.7–6.4.9: check runs parse, semantic analyses and lowering
// validation over one source file; a clean file exits 0 with no
// diagnostics on stdout.
func TestCheckCleanFileExitsZero(t *testing.T) {
	path := writeTemp(t, "var x int = 1\nx\n")
	code, stdout, stderr := runCLI([]string{"check", path})
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want clean success", code, stdout, stderr)
	}
}

// §6.12.1: the human diagnostic line is
// file:line:col: severity[code]: message — the whole reason the CLI
// needs the offset→line/col mapping.
func TestCheckErrorLineColFormat(t *testing.T) {
	path := writeTemp(t, "var user User?\nvar f = func() {\nuser.save()\n}\n")
	code, stdout, _ := runCLI([]string{"check", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	// The primary RBInit read sits at byte 32 — line 3, column 1.
	if !strings.Contains(stdout, "test.anuy:3:1: error[ANUY3001]") {
		t.Fatalf("output misses the line/col diagnostic:\n%s", stdout)
	}
	if !strings.Contains(stdout, "warning[ANUY") {
		// nothing: related diagnostics may or may not appear; the primary
		// line is the pin above.
		_ = strings.TrimSpace(stdout)
	}
}

// §6.12.20: at least one Error diagnostic → exit 1.
func TestCheckErrorSeverityExitsOne(t *testing.T) {
	path := writeTemp(t, "x\n")
	code, stdout, _ := runCLI([]string{"check", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "error[ANUY2001]") {
		t.Fatalf("output misses ANUY2001:\n%s", stdout)
	}
}

// §6.12.20: advisory (Warning) diagnostics do not fail check — exit 0.
func TestCheckAdvisoryExitsZero(t *testing.T) {
	path := writeTemp(t, "var err error? = f()\n")
	code, stdout, _ := runCLI([]string{"check", path})
	if code != 0 {
		t.Fatalf("code = %d, want 0 (advisory does not fail)", code)
	}
	if !strings.Contains(stdout, "warning[ANUY5001]") {
		t.Fatalf("output misses the ANUY5001 warning:\n%s", stdout)
	}
}

// §6.12.19: -json emits the machine diagnostics document.
func TestCheckJSONFlag(t *testing.T) {
	path := writeTemp(t, "x\n")
	code, stdout, _ := runCLI([]string{"check", "-json", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	var doc struct {
		Version     int `json:"version"`
		Diagnostics []struct {
			Code string `json:"code"`
		} `json:"diagnostics"`
	}
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if doc.Version != 1 || len(doc.Diagnostics) == 0 || doc.Diagnostics[0].Code != "ANUY2001" {
		t.Fatalf("document = %+v, want version 1 with ANUY2001", doc)
	}
}

// §6.12.20: usage failures exit 2.
func TestCheckUsageExitsTwo(t *testing.T) {
	if code, _, _ := runCLI([]string{"check"}); code != 2 {
		t.Fatalf("no-file code = %d, want 2", code)
	}
	if code, _, _ := runCLI([]string{"check", "/nonexistent/xyz.anuy"}); code != 2 {
		t.Fatalf("missing-file code = %d, want 2", code)
	}
	if code, _, _ := runCLI([]string{"nonsense"}); code != 2 {
		t.Fatalf("unknown subcommand code = %d, want 2", code)
	}
}

// §6.4.8/§6.4.9: lowering validation belongs to check — a soundness
// boundary reject (seen-map callback param, story 55) is a check
// failure (exit 1).
func TestCheckLoweringRejectExitsOne(t *testing.T) {
	path := writeTemp(t, "type User struct {\nid int\nlink *User\n}\nfunc run() {\ngoCall(func(u User) {\nuse(u)\n})\n}\n")
	code, stdout, _ := runCLI([]string{"check", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (lowering validation failure)", code)
	}
	if !strings.Contains(stdout, "seen-map wrapper slice") {
		t.Fatalf("output misses the wrapper reject reason:\n%s", stdout)
	}
}

// --- Story 58 (RFC-010 §6.4.1–6.4.2, §6.4.5–6.4.6, §6.4.11–6.4.12) ---

// emit-go materializes the Go ABI view; default output is stdout.
func TestEmitGoStdout(t *testing.T) {
	path := writeTemp(t, "var x int = 1\nx\n")
	code, stdout, stderr := runCLI([]string{"emit-go", path})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "package fixture") || !strings.Contains(stdout, "func Run()") {
		t.Fatalf("emitted Go misses the generated shape:\n%s", stdout)
	}
}

// emit-go -o writes the generated Go to the given file.
func TestEmitGoOutputFile(t *testing.T) {
	path := writeTemp(t, "var x int = 1\nx\n")
	out := filepath.Join(t.TempDir(), "out.go")
	code, _, stderr := runCLI([]string{"emit-go", "-o", out, path})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), "func Run()") {
		t.Fatalf("out file = %q, err = %v", data, err)
	}
}

// build materializes <base>.anuy.go next to the source (§6.4.2 v1:
// the final go build belongs to the user's Go toolchain).
func TestBuildWritesGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "hello.anuy")
	if err := os.WriteFile(path, []byte("var x int = 1\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI([]string{"build", path})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	data, err := os.ReadFile(filepath.Join(dir, "hello.anuy.go"))
	if err != nil || !strings.Contains(string(data), "func Run()") {
		t.Fatalf("built file = %q, err = %v", data, err)
	}
}

// build -o places the generated file into the given directory.
func TestBuildOutputDir(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "hello.anuy")
	if err := os.WriteFile(path, []byte("var x int = 1\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(dir, "gen")
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI([]string{"build", "-o", outDir, path})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(outDir, "hello.anuy.go")); err != nil {
		t.Fatalf("built file missing in -o dir: %v", err)
	}
}

// build reports diagnostics with exit 1 (same §6.12.20 policy).
func TestBuildDiagnosticsExitOne(t *testing.T) {
	path := writeTemp(t, "x\n")
	code, stdout, _ := runCLI([]string{"build", path})
	if code != 1 || !strings.Contains(stdout, "error[ANUY2001]") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

// run compiles and executes the program in a temp module; ANUY_REPLACE_ROOT
// points the module at the repository (dev workflow). The program exit code
// passes through.
func TestRunExecutesProgram(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUY_REPLACE_ROOT", root)
	path := writeTemp(t, "var x int = 1\nx = x + 41\nx\n")
	code, stdout, stderr := runCLI([]string{"run", path})
	if code != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// §6.4.6: program args after `--` are passed through, not parsed as
// compiler flags.
func TestRunSeparatesProgramArgs(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUY_REPLACE_ROOT", root)
	path := writeTemp(t, "var x = 1\nx\n")
	code, _, _ := runCLI([]string{"run", path, "--", "--port", "8080"})
	if code != 0 {
		t.Fatalf("code = %d, want 0 (program args must not be compiler flags)", code)
	}
}

// run reports semantic diagnostics with exit 1 before compiling.
func TestRunDiagnosticsExitOne(t *testing.T) {
	path := writeTemp(t, "x\n")
	code, stdout, _ := runCLI([]string{"run", path})
	if code != 1 || !strings.Contains(stdout, "error[ANUY2001]") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

// --- Story 59 (RFC-010 §6.4.2 п. 4–5, §6.9.8) ---

// writeModule creates the module context the build invocation needs
// (§6.4.2 item 4: Go builds inside the user's module).
func writeModule(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module fixture\n\ngo 1.24\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

// §6.4.2 item 4: build invokes the Go toolchain over the materialized
// file; success is silent with exit 0 (cmd/go convention).
func TestBuildGoInvocationSuccess(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "hello.anuy")
	if err := os.WriteFile(path, []byte("var x int = 1\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI([]string{"build", path})
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("code=%d stdout=%q stderr=%q, want silent success", code, stdout, stderr)
	}
}

// §6.4.2 item 5/§6.9.8: a Go-side failure surfaces in .anuy coordinates —
// the //line directives make the toolchain itself blame the source.
// `range` is not reserved in Anuy but is a Go keyword: the collision is
// invisible to check (§6.4.9) and only go build sees it — exit 1 (the
// §6.12.20 Error class).
func TestBuildGoCollisionRemapsToSource(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "range.anuy")
	if err := os.WriteFile(path, []byte("var range = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI([]string{"build", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (Go build failure)", code)
	}
	if combined := stdout + stderr; !strings.Contains(combined, "range.anuy:") {
		t.Fatalf("output misses the remapped .anuy position:\n%s", combined)
	}
}

// §6.12.20 (v7): no module context — usage/internal failure, exit 2; the
// environment is not a source diagnostic. Nothing is materialized.
func TestBuildNoModuleExitsTwo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.anuy")
	if err := os.WriteFile(path, []byte("var x int = 1\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI([]string{"build", path})
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "go.mod") {
		t.Fatalf("stderr misses the module hint:\n%s", stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "hello.anuy.go")); err == nil {
		t.Fatal("generated file materialized despite the environment failure")
	}
}

// §6.12.20 (v7): the Go toolchain itself unavailable — exit 2.
func TestBuildNoGoToolchainExitsTwo(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "hello.anuy")
	if err := os.WriteFile(path, []byte("var x int = 1\nx\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", "")
	if code, _, stderr := runCLI([]string{"build", path}); code != 2 {
		t.Fatalf("code = %d, want 2 (stderr: %q)", code, stderr)
	}
}
