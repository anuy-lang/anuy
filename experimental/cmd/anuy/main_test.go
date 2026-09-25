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
	// The callee is declared (story 64: the type-check stage resolves
	// initializer callees) - the pin stays on the advisory-vs-error
	// distinction.
	path := writeTemp(t, "func f() error? {\nreturn nil\n}\nvar err error? = f()\n")
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

// §6.4.13 (story 60): the collision is a check-stage diagnostic - the
// §6.12.1 human line with the §6.2.15 code, exit 1.
func TestCheckGoReservedIdentifier(t *testing.T) {
	path := writeTemp(t, "var range = 1\n")
	code, stdout, _ := runCLI([]string{"check", path})
	if code != 1 || !strings.Contains(stdout, "test.anuy:1:1: error[ANUY9001]") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
	}
}

// §6.12.19: the machine document carries the code.
func TestCheckGoReservedIdentifierJSON(t *testing.T) {
	path := writeTemp(t, "var range = 1\n")
	code, stdout, _ := runCLI([]string{"check", "-json", path})
	if code != 1 || !strings.Contains(stdout, "ANUY9001") {
		t.Fatalf("code = %d, stdout = %q", code, stdout)
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

// build materializes <base>.anuy.go next to the source (§6.4.2: the
// package build runs over the materialized package directory).
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

// --- Story 61 (RFC-010 §6.1.7, §6.4.2 п. 4, §6.4.4; RFC-015 §6.1 v3) ---

// run shapes the declared package into a main program - the shaping
// targets the actual package line, not a fixed spelling.
func TestRunWithDeclaredPackage(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUY_REPLACE_ROOT", root)
	path := writeTemp(t, "package demo\nvar x = 1\nx\n")
	if code, _, stderr := runCLI([]string{"run", path}); code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
}

// §6.4.2 п. 4: build invokes the Go build over the logical mixed package
// - the sibling .go sharing the declared package compiles together with
// the generated file.
func TestBuildMixedPackage(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "server.anuy")
	if err := os.WriteFile(path, []byte("package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\nAdd(1, 2)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(dir, "use.go")
	if err := os.WriteFile(sibling, []byte("package server\n\n// Use consumes the generated API inside the same package.\nfunc Use() int { return Add(2, 3) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if code, stdout, stderr := runCLI([]string{"build", path}); code != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// The package build sees the whole package: a broken sibling fails the
// build with the honest Go diagnostic (file-mode v2 could not see it).
func TestBuildMixedPackageBrokenSibling(t *testing.T) {
	dir := t.TempDir()
	writeModule(t, dir)
	path := filepath.Join(dir, "server.anuy")
	if err := os.WriteFile(path, []byte("package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\nAdd(1, 2)\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(dir, "use.go")
	if err := os.WriteFile(sibling, []byte("package server\n\nfunc Use() int { return Undefined }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, stderr := runCLI([]string{"build", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (broken sibling), stderr = %q", code, stderr)
	}
	if !strings.Contains(stderr, "use.go") {
		t.Fatalf("stderr misses the sibling diagnostic:\n%s", stderr)
	}
}

// §6.4.4: materialization outside the package dir is incompatible with
// the package-mode build - the -o flag is rejected explicitly.
func TestBuildRejectsOutputDir(t *testing.T) {
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
	if code, _, _ := runCLI([]string{"build", "-o", outDir, path}); code != 2 {
		t.Fatalf("code = %d, want 2 (explicit -o rejection)", code)
	}
}

// --- Story 62 (RFC-015 §6.1 v4, §6.4.1; RFC-011 v9) ---

// writePackage writes two files of one package directory.
func writePackage(t *testing.T, aSource, bSource string) string {
	t.Helper()
	dir := t.TempDir()
	for name, src := range map[string]string{"a.anuy": aSource, "b.anuy": bSource} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// §6.4.1: a directory argument checks the whole package. Clause mismatch
// across the directory's files is ANUY9002 (RFC-011 v9) - the generated
// Go would not form a package - blamed on the mismatching file, exit 1.
func TestCheckDirectoryMismatch(t *testing.T) {
	dir := writePackage(t,
		"package server\nvar x int = 1\nx\n",
		"package client\nvar y int = 2\ny\n")
	code, stdout, stderr := runCLI([]string{"check", dir})
	if code != 1 {
		t.Fatalf("code = %d, want 1, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "b.anuy:1:1: error[ANUY9002]") {
		t.Fatalf("output misses the mismatch diagnostic:\n%s", stdout)
	}
}

// A directory without .anuy files is a usage failure.
func TestCheckDirectoryEmpty(t *testing.T) {
	if code, _, stderr := runCLI([]string{"check", t.TempDir()}); code != 2 {
		t.Fatalf("code = %d, want 2, stderr = %q", code, stderr)
	}
}

// Diagnostics in package mode carry the source file (§6.12.19): an
// undefined read in b.anuy is blamed on b.anuy, not on the directory or
// the first file.
func TestCheckDirectoryFileAttribution(t *testing.T) {
	dir := writePackage(t,
		"package server\nvar x int = 1\nx\n",
		"package server\ny\n")
	code, stdout, _ := runCLI([]string{"check", dir})
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(stdout, "b.anuy:2:1: error[ANUY2001]") {
		t.Fatalf("output misses the b.anuy-attributed diagnostic:\n%s", stdout)
	}
}

// §6.1 RFC-015: package-private declarations are reachable from any file
// of the same package - b.anuy calls a.anuy's Add without imports
// (§6.10.2 RFC-010: declaration order is not observable). The call must
// be a statement: package-level initializer calls are unchecked in the
// experimental layer (pre-existing gap, story-62 findings).
func TestCheckDirectoryCrossFile(t *testing.T) {
	dir := writePackage(t,
		"package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\n",
		"package server\nAdd(1, 2)\n")
	code, stdout, stderr := runCLI([]string{"check", dir})
	if code != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
}

// The merged namespace rejects duplicate declarations across files
// (§6.16 RFC-015: namespace collisions MUST be rejected).
func TestCheckDirectoryDuplicateDecl(t *testing.T) {
	dir := writePackage(t,
		"package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\n",
		"package server\nfunc Add(a int, b int) int {\nreturn a\n}\n")
	code, stdout, _ := runCLI([]string{"check", dir})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (duplicate across files)", code)
	}
	if !strings.Contains(stdout, "error[ANUY2003]") {
		t.Fatalf("output misses the redeclaration diagnostic:\n%s", stdout)
	}
}

// build <dir> materializes ONE merged generated file per package
// (§6.2.6 RFC-010: physical placement is not a semantic property): all
// declarations with //line anchors to their sources and a single Run -
// two per-file Run bodies would collide in one package. The mixed
// package (two .anuy + handwritten sibling) compiles as one unit.
// Cross-file sharing is through declarations: package-level vars are
// Run-locals of their own file in the experimental layer.
func TestBuildDirectoryMergedPackage(t *testing.T) {
	dir := writePackage(t,
		"package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\n",
		"package server\nfunc Total() int {\nreturn Add(1, 2)\n}\nTotal()\n")
	sibling := filepath.Join(dir, "use.go")
	if err := os.WriteFile(sibling, []byte("package server\n\n// Use consumes the generated API of both files.\nfunc Use() int { return Add(2, Total()) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeModule(t, dir)
	code, stdout, stderr := runCLI([]string{"build", dir})
	if code != 0 {
		t.Fatalf("code = %d, stdout = %q, stderr = %q", code, stdout, stderr)
	}
	merged, err := os.ReadFile(filepath.Join(dir, "server.anuy.go"))
	if err != nil {
		t.Fatalf("merged materialization missing: %v", err)
	}
	text := string(merged)
	// Directives carry the command-line path spelling (§6.9.3) - the
	// test passes an absolute directory.
	if !strings.Contains(text, "//line "+filepath.Join(dir, "a.anuy")+":") ||
		!strings.Contains(text, "//line "+filepath.Join(dir, "b.anuy")+":") {
		t.Fatalf("merged file misses the per-file //line anchors:\n%s", text)
	}
	if strings.Count(text, "func Run()") != 1 {
		t.Fatalf("merged file carries %d Run declarations, want one:\n%s", strings.Count(text, "func Run()"), text)
	}
	for _, stale := range []string{"a.anuy.go", "b.anuy.go"} {
		if _, err := os.Stat(filepath.Join(dir, stale)); err == nil {
			t.Fatalf("per-file materialization %s exists - the package generates one file", stale)
		}
	}
}

// §6.15 RFC-015 (story 63): imports give programs the stdout channel -
// run verifies the printed output, not only exit codes (closes F-58-4).
func TestRunPrintsStdout(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUY_REPLACE_ROOT", root)
	path := writeTemp(t, "package main\nimport \"fmt\"\nfmt.Println(\"hello from anuy\")\n")
	code, stdout, stderr := runCLI([]string{"run", path})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "hello from anuy") {
		t.Fatalf("stdout misses the program output:\n%q", stdout)
	}
}

// §6.4.8 (story 64): the Go type-check stage - an unknown callee in an
// initializer fails check through the toolchain, in .anuy coordinates
// (closes F-62-1: the check stage no longer silently skips what go build
// catches).
func TestCheckGoStageCatchesUnknownCallee(t *testing.T) {
	path := writeTemp(t, "package p\nvar x = Ghost()\nx\n")
	code, stdout, stderr := runCLI([]string{"check", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (type-check failure)", code)
	}
	if combined := stdout + stderr; !strings.Contains(combined, "test.anuy:2:") || !strings.Contains(combined, "undefined: Ghost") {
		t.Fatalf("output misses the remapped diagnostic:\n%s", combined)
	}
}

// The type-check stage requires the Go toolchain: unavailable toolchain
// is a usage/internal failure (§6.12.20 v7), not a source diagnostic.
func TestCheckNoGoToolchainExitsTwo(t *testing.T) {
	path := writeTemp(t, "var x int = 1\nx\n")
	t.Setenv("PATH", "")
	if code, _, stderr := runCLI([]string{"check", path}); code != 2 {
		t.Fatalf("code = %d, want 2, stderr = %q", code, stderr)
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

// §6.4.13 (story 60): the Go-keyword collision is caught at check - the
// coded diagnostic precedes materialization (§6.4.9). The //line remap
// machinery stays covered by the lowering directive pins and the
// build-success e2e (story 59 evidence).
func TestBuildGoCollisionCaughtAtCheck(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "range.anuy")
	if err := os.WriteFile(path, []byte("var range = 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runCLI([]string{"build", path})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (check failure)", code)
	}
	if !strings.Contains(stdout, "error[ANUY9001]") {
		t.Fatalf("output misses the coded diagnostic:\n%s%s", stdout, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "range.anuy.go")); err == nil {
		t.Fatal("generated file materialized despite the check failure")
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

// --- Story 65 (§6.4.1, §6.4.5, §6.4.7–6.4.8, §6.12.2) ---

// §6.4.5: run executes the package program; the union imports give the
// stdout channel.
func TestRunDirectoryPrintsStdout(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("ANUY_REPLACE_ROOT", root)
	dir := writePackage(t,
		"package server\nfunc Total() int {\nreturn 40 + 2\n}\n",
		"package server\nimport \"fmt\"\nvar t = Total()\nfmt.Println(\"total:\", t)\n")
	code, stdout, stderr := runCLI([]string{"run", dir})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "total: 42") {
		t.Fatalf("stdout misses the package output:\n%q", stdout)
	}
}

// §6.4.11: emit-go <dir> prints the merged ABI view - anchors of both
// files, one Run.
func TestEmitGoDirectoryMerged(t *testing.T) {
	dir := writePackage(t,
		"package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\n",
		"package server\nfunc Total() int {\nreturn Add(1, 2)\n}\nTotal()\n")
	code, stdout, stderr := runCLI([]string{"emit-go", dir})
	if code != 0 {
		t.Fatalf("code = %d, stderr = %q", code, stderr)
	}
	if !strings.Contains(stdout, "//line "+filepath.Join(dir, "a.anuy")+":") ||
		!strings.Contains(stdout, "//line "+filepath.Join(dir, "b.anuy")+":") {
		t.Fatalf("merged view misses the per-file anchors:\n%s", stdout)
	}
	if strings.Count(stdout, "func Run()") != 1 {
		t.Fatalf("merged view carries %d Run declarations, want one:\n%s", strings.Count(stdout, "func Run()"), stdout)
	}
}

// §6.4.8 (story 65): check <dir> runs the go type-check stage - an
// unknown callee in a package file fails check, remapped to its .anuy
// line (closes the F-64-3 boundary for directories). The transient
// materialization is cleaned up even on failure (§6.12.2).
func TestCheckDirectoryGoStage(t *testing.T) {
	dir := writePackage(t,
		"package server\nvar x = Ghost()\nx\n",
		"package server\nfunc Total() int {\nreturn 1\n}\n")
	writeModule(t, dir)
	code, stdout, stderr := runCLI([]string{"check", dir})
	if code != 1 {
		t.Fatalf("code = %d, want 1 (type-check failure)", code)
	}
	if combined := stdout + stderr; !strings.Contains(combined, "a.anuy:2:") || !strings.Contains(combined, "undefined: Ghost") {
		t.Fatalf("output misses the remapped diagnostic:\n%s", combined)
	}
	if _, err := os.Stat(filepath.Join(dir, "server.anuy.go")); err == nil {
		t.Fatal("transient materialization left behind after the failure")
	}
}

// A clean mixed package passes the go stage silently - the handwritten
// sibling consumes the generated API of both files.
func TestCheckDirectoryMixedClean(t *testing.T) {
	dir := writePackage(t,
		"package server\nfunc Add(a int, b int) int {\nreturn a + b\n}\n",
		"package server\nfunc Total() int {\nreturn Add(1, 2)\n}\nTotal()\n")
	sibling := filepath.Join(dir, "use.go")
	if err := os.WriteFile(sibling, []byte("package server\n\nfunc Use() int { return Add(2, Total()) }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	writeModule(t, dir)
	code, stdout, stderr := runCLI([]string{"check", dir})
	if code != 0 || stdout != "" || stderr != "" {
		t.Fatalf("code = %d, stdout = %q, stderr = %q, want silent success", code, stdout, stderr)
	}
}
