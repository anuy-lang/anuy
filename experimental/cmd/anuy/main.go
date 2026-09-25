// Command anuy is the experimental CLI of the validation layer
// (ADR-0007 L0). Implemented slices of RFC-010 §6.4: `anuy check`
// (§6.4.7–6.4.9), `anuy build` (§6.4.1–6.4.2: package-mode Go-build
// invocation over the materialized mixed package, §6.9.8 //line remap),
// `anuy emit-go` (§6.4.11–6.4.12) and `anuy run` (§6.4.5–6.4.6,
// temp-module). Diagnostics render per RFC-011 §6.12.1/§6.12.19 with
// the §6.12.20 exit codes.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/anuy-lang/anuy/experimental/integration"
	"github.com/anuy-lang/anuy/experimental/lowering"
	"github.com/anuy-lang/anuy/experimental/parser"
	"github.com/anuy-lang/anuy/internal/semantic"
)

// Exit codes (RFC-011 §6.12.20): 0 — no Error diagnostics; 1 — at
// least one Error diagnostic; 2 — usage or internal failure.
const (
	exitOK    = 0
	exitDiag  = 1
	exitUsage = 2

	// anuyabiVersion is the module version the run command requires in
	// its temporary module. Aligned with the released tags (ADR-0014:
	// the compiler and anuyabi are versioned by the same repository
	// tags).
	anuyabiVersion = "v0.1.0"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: anuy <command> [flags] — commands: check, build, emit-go, run")
		return exitUsage
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	case "build":
		return runBuild(args[1:], stdout, stderr)
	case "emit-go":
		return runEmitGo(args[1:], stdout, stderr)
	case "run":
		return runRun(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q — commands: check, build, emit-go, run\n", args[0])
		return exitUsage
	}
}

// checkOutcome is the outcome of the check stages (RFC-010 §6.4.8) over
// one source file: the parse/semantic result, the generated Go, and a
// rendered failure when a stage rejected the source.
type checkOutcome struct {
	source    string
	result    integration.Result
	generated string

	// parseFailure is the rendered parse diagnostic line (codes from the
	// registry); lowerFailure is the rendered lowering-validation reject;
	// usageErr is a usage/internal failure. Exactly one failure field is
	// set when a stage rejected the source.
	parseFailure string
	lowerFailure string
	usageErr     string

	hasErrorDiag bool
}

// fail reports the rendered failure and its exit class, if any.
func (o checkOutcome) fail() (string, bool) {
	if o.usageErr != "" {
		return o.usageErr, true
	}
	if o.parseFailure != "" {
		return o.parseFailure, false
	}
	if o.lowerFailure != "" {
		return o.lowerFailure, false
	}
	return "", false
}

// checkStages runs the check stages of §6.4.8 in the experimental-slice
// scope: parse + semantic analyses (integration) and lowering
// validation (lowering), per §6.4.9.
func checkStages(path string) checkOutcome {
	var out checkOutcome
	source, err := os.ReadFile(path)
	if err != nil {
		out.usageErr = err.Error()
		return out
	}
	out.source = string(source)

	result, aerr := integration.AnalyzeSource(out.source)
	if aerr != nil {
		// Parse errors carry registry codes — they are user-facing
		// diagnostics, not tool failures.
		if perr, ok := aerr.(*parser.Error); ok {
			line, col := lineCol(out.source, perr.Offset)
			out.parseFailure = fmt.Sprintf("%s:%d:%d: %s[%s]: %s", path, line, col,
				strings.ToLower(string(perr.Severity)), perr.Code, perr.Message)
			out.hasErrorDiag = true
			return out
		}
		out.usageErr = aerr.Error()
		return out
	}
	out.result = result

	// Lowering validation belongs to check (§6.4.8/§6.4.9): a
	// soundness-boundary reject is a user-facing check failure.
	text, lerr := lowering.LowerFile(path, out.source)
	if lerr != nil {
		out.lowerFailure = fmt.Sprintf("%s: error: %v", path, lerr)
		return out
	}
	out.generated = text
	out.hasErrorDiag = hasErrorSeverity(result.Diagnostics)
	return out
}

// stageFailure renders the stage failures (parse, lowering reject,
// usage) and returns the §6.12.20 exit code with handled=true.
// Semantic diagnostics are not rendered here — each command renders
// them in its own output mode.
func (o checkOutcome) stageFailure(path string, stdout, stderr io.Writer) (int, bool) {
	if o.usageErr != "" {
		fmt.Fprintln(stderr, "anuy:", o.usageErr)
		return exitUsage, true
	}
	if o.parseFailure != "" {
		fmt.Fprintln(stdout, o.parseFailure)
		return exitDiag, true
	}
	if o.lowerFailure != "" {
		fmt.Fprintln(stdout, o.lowerFailure)
		return exitDiag, true
	}
	return exitOK, false
}

func runCheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("anuy check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	jsonOut := fs.Bool("json", false, "emit the machine diagnostics JSON document (RFC-011 §6.12.19)")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: anuy check [-json] <file.anuy | dir>")
		return exitUsage
	}
	target := fs.Arg(0)
	if isPackagePattern(target) {
		return runCheckPattern(target, *jsonOut, stdout, stderr)
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return runCheckDir(target, *jsonOut, stdout, stderr)
	}
	path := target
	out := checkStages(path)
	if code, handled := out.stageFailure(path, stdout, stderr); handled {
		return code
	}

	if *jsonOut {
		data, jerr := integration.DiagnosticsJSON(out.result)
		if jerr != nil {
			fmt.Fprintln(stderr, "anuy check:", jerr)
			return exitUsage
		}
		stdout.Write(data)
		stdout.Write([]byte("\n"))
	} else {
		printDiagnostics(stdout, path, out.source, out.result.Diagnostics)
	}

	if out.hasErrorDiag {
		return exitDiag
	}
	// §6.4.8: the Go package/type compatibility check stage (story 64) -
	// the generated Go compiles against the toolchain before check
	// passes.
	return checkGoStage(out.generated, stdout, stderr)
}

// checkGoStage runs the §6.4.8 Go type-check stage (story 64): the
// generated Go compiles in a temporary module - no execution, no
// artifacts (§6.12.2). Go diagnostics come back in .anuy coordinates
// through the //line directives (§6.9.8).
func checkGoStage(generated string, stdout, stderr io.Writer) int {
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(stderr, "anuy check: the Go toolchain is required for the type-check stage:", err)
		return exitUsage
	}
	dir, err := os.MkdirTemp("", "anuy-check-")
	if err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	defer os.RemoveAll(dir)
	if err := writeTempModule(dir); err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	if err := os.WriteFile(filepath.Join(dir, "fixture.go"), []byte(generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return exitDiag
		}
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	return exitOK
}

// packageFailure is the classified outcome of the package check stages:
// a rendered parse diagnostic line (stdout, exitDiag) or a usage/internal
// message (stderr, exitUsage).
type packageFailure struct {
	code int
	msg  string
	diag string
}

// analyzePackage runs the check stages over a package directory (story
// 62): discovery, parse, clause uniformity, merged analysis.
func analyzePackage(dir string) ([]integration.SourceFile, integration.Result, *packageFailure) {
	files, code, msg := readPackage(dir)
	if files == nil {
		return nil, integration.Result{}, &packageFailure{code: code, msg: msg}
	}
	result, err := integration.AnalyzeFiles(files)
	if err != nil {
		var fe *integration.FileError
		if errors.As(err, &fe) {
			var source string
			for _, f := range files {
				if f.Path == fe.Path {
					source = f.Source
				}
			}
			line, col := lineCol(source, fe.Err.Offset)
			return files, result, &packageFailure{code: exitDiag, diag: fmt.Sprintf("%s:%d:%d: %s[%s]: %s", fe.Path, line, col,
				strings.ToLower(string(fe.Err.Severity)), fe.Err.Code, fe.Err.Message)}
		}
		return files, result, &packageFailure{code: exitUsage, msg: err.Error()}
	}
	return files, result, nil
}

// isPackagePattern reports whether the target is a package pattern
// (§6.4.1): the `/...` suffix or the bare `...`.
func isPackagePattern(target string) bool {
	return target == "..." || strings.HasSuffix(target, "/...")
}

// matchPackages expands a package pattern (§6.4.1) to package
// directories: every directory below the pattern root containing at
// least one .anuy file, in sorted order. Go-ignored names (dot and
// underscore prefixes, testdata) do not match.
func matchPackages(pattern string) ([]string, error) {
	root := strings.TrimSuffix(strings.TrimSuffix(pattern, "..."), "/")
	if root == "" {
		root = "."
	}
	var dirs []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && d.IsDir() {
			if name := d.Name(); strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" {
				return filepath.SkipDir
			}
		}
		if matches, _ := filepath.Glob(filepath.Join(path, "*.anuy")); len(matches) > 0 {
			dirs = append(dirs, path)
		}
		return nil
	})
	sort.Strings(dirs)
	return dirs, err
}

// runCheckPattern implements `anuy check <pattern>` (story 66, §6.4.1):
// every matched package is checked - diagnostics aggregate into one
// document with file attribution (§6.12.19), the go stage runs per
// package, and any Error fails the run while the remaining packages are
// still processed. A pattern matching no packages succeeds silently
// (cmd/go convention).
func runCheckPattern(pattern string, jsonOut bool, stdout, stderr io.Writer) int {
	dirs, err := matchPackages(pattern)
	if err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	if len(dirs) == 0 {
		return exitOK
	}
	code := exitOK
	var all []semantic.Diagnostic
	for _, dir := range dirs {
		files, result, failure := analyzePackage(dir)
		if failure != nil {
			if failure.diag != "" {
				fmt.Fprintln(stdout, failure.diag)
			} else {
				fmt.Fprintf(stderr, "anuy check: %s\n", failure.msg)
			}
			code = worstCode(code, failure.code)
			continue
		}
		all = append(all, result.Diagnostics...)
		if !jsonOut {
			printPackageDiagnostics(stdout, files, result.Diagnostics)
		}
		stage := exitOK
		if hasErrorSeverity(result.Diagnostics) {
			stage = exitDiag
		} else {
			stage = checkPackageGoStage(files, dir, stdout, stderr)
		}
		code = worstCode(code, stage)
	}
	if jsonOut {
		data, jerr := integration.DiagnosticsJSON(integration.Result{Diagnostics: all})
		if jerr != nil {
			fmt.Fprintln(stderr, "anuy check:", jerr)
			return exitUsage
		}
		stdout.Write(data)
		stdout.Write([]byte("\n"))
	}
	return code
}

// runBuildPattern implements `anuy build <pattern>` (story 66, §6.4.1):
// every matched package builds (persistent materialization - build's
// product); any failure fails the run while the remaining packages are
// still built. A pattern matching no packages succeeds silently.
func runBuildPattern(pattern string, stdout, stderr io.Writer) int {
	dirs, err := matchPackages(pattern)
	if err != nil {
		fmt.Fprintln(stderr, "anuy build:", err)
		return exitUsage
	}
	if len(dirs) == 0 {
		return exitOK
	}
	code := exitOK
	for _, dir := range dirs {
		code = worstCode(code, runBuildDir(dir, stdout, stderr))
	}
	return code
}

// worstCode returns the more severe exit code (usage/internal outranks a
// diagnostic failure).
func worstCode(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// runCheckDir implements `anuy check <dir>` (story 62, §6.4.1): the
// directory's .anuy files are one package (§6.1.5 RFC-010). Diagnostics
// carry the source file (§6.12.19).
func runCheckDir(dir string, jsonOut bool, stdout, stderr io.Writer) int {
	files, result, failure := analyzePackage(dir)
	if failure != nil {
		return reportPackageFailure(failure, "check", stdout, stderr)
	}

	if jsonOut {
		data, jerr := integration.DiagnosticsJSON(result)
		if jerr != nil {
			fmt.Fprintln(stderr, "anuy check:", jerr)
			return exitUsage
		}
		stdout.Write(data)
		stdout.Write([]byte("\n"))
	} else {
		printPackageDiagnostics(stdout, files, result.Diagnostics)
	}
	if hasErrorSeverity(result.Diagnostics) {
		return exitDiag
	}
	// §6.4.8: the go type-check stage (story 65) - transient
	// materialization of the merged package, removed on every path
	// (§6.12.2).
	return checkPackageGoStage(files, dir, stdout, stderr)
}

// checkPackageGoStage runs the §6.4.8 go type-check stage for a package
// directory (story 65): the merged generated file materializes next to
// the sources so the mixed package compiles in the user's module
// context, and is removed on every path - check leaves no artifacts
// (§6.12.2). Go diagnostics come back in .anuy coordinates through the
// //line directives (§6.9.8).
func checkPackageGoStage(files []integration.SourceFile, dir string, stdout, stderr io.Writer) int {
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(stderr, "anuy check: the Go toolchain is required for the type-check stage:", err)
		return exitUsage
	}
	if !inModule(dir) {
		fmt.Fprintln(stderr, "anuy check: no go.mod found above", dir, "- the type-check stage runs inside a Go module (go mod init)")
		return exitUsage
	}
	generated, err := lowerPackageFiles(files)
	if err != nil {
		fmt.Fprintf(stdout, "%s: error: %v\n", dir, err)
		return exitDiag
	}
	target := filepath.Join(dir, packageName(generated)+".anuy.go")
	if err := os.WriteFile(target, []byte(generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	defer os.Remove(target)
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return exitDiag
		}
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	return exitOK
}

// readPackage discovers the .anuy files of one package directory
// (§6.1.5 RFC-010) in deterministic (sorted) order.
func readPackage(dir string) ([]integration.SourceFile, int, string) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.anuy"))
	if err != nil {
		return nil, exitUsage, err.Error()
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, exitUsage, dir + " has no .anuy files"
	}
	files := make([]integration.SourceFile, 0, len(paths))
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, exitUsage, err.Error()
		}
		files = append(files, integration.SourceFile{Path: p, Source: string(data)})
	}
	return files, exitOK, ""
}

// printPackageDiagnostics renders diagnostics with per-file coordinates
// (§6.12.19: the span names its source file).
func printPackageDiagnostics(w io.Writer, files []integration.SourceFile, diags []semantic.Diagnostic) {
	sources := make(map[string]string, len(files))
	for _, f := range files {
		sources[f.Path] = f.Source
	}
	var print func(diags []semantic.Diagnostic)
	print = func(diags []semantic.Diagnostic) {
		for _, d := range diags {
			line, col := lineCol(sources[d.Span.File], d.Span.Start)
			message, _ := semantic.Message(d.Code)
			fmt.Fprintf(w, "%s:%d:%d: %s[%s]: %s\n", d.Span.File, line, col,
				strings.ToLower(string(d.Severity)), d.Code, message)
			print(d.Related)
		}
	}
	print(diags)
}

// runBuild implements `anuy build` (§6.4.1–6.4.2): check, materialize
// the generated Go next to the source and invoke the Go build over the
// logical mixed package (§6.4.2 item 4, story 61): the package directory
// compiles as one unit - generated files and handwritten siblings. Go
// errors come back in .anuy coordinates through the //line directives
// (§6.9.8); success is silent (cmd/go convention).
func runBuild(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("anuy build", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: anuy build <file.anuy>")
		return exitUsage
	}
	path := fs.Arg(0)
	if isPackagePattern(path) {
		return runBuildPattern(path, stdout, stderr)
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return runBuildDir(path, stdout, stderr)
	}
	out := checkStages(path)
	if code, handled := out.stageFailure(path, stdout, stderr); handled {
		return code
	}
	if out.hasErrorDiag {
		// Semantic errors: report instead of materializing the file.
		printDiagnostics(stdout, path, out.source, out.result.Diagnostics)
		return exitDiag
	}

	// Environment pre-check happens before materialization: a doomed run
	// must not leave generated artifacts behind (§6.12.2).
	dir := filepath.Dir(path)
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(stderr, "anuy build: the Go toolchain is required:", err)
		return exitUsage
	}
	if !inModule(dir) {
		fmt.Fprintln(stderr, "anuy build: no go.mod found above", dir, "- anuy build runs inside a Go module (go mod init)")
		return exitUsage
	}
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	target := filepath.Join(dir, base+".anuy.go")
	if err := os.WriteFile(target, []byte(out.generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy build:", err)
		return exitUsage
	}

	// Package-mode build (§6.4.2 item 4): the whole package - generated
	// file and handwritten siblings - compiles as one unit.
	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			// Go-toolchain diagnostics are the §6.12.20 Error class
			// (v7) - already in .anuy coordinates via //line.
			return exitDiag
		}
		fmt.Fprintln(stderr, "anuy build:", err)
		return exitUsage
	}
	return exitOK
}

// writeTempModule writes the run/check temporary module (story 58/64):
// the published anuyabi satisfies the generated imports;
// ANUY_REPLACE_ROOT redirects to a repository checkout for the
// development workflow.
func writeTempModule(dir string) error {
	gomod := "module runtmp\n\ngo 1.24\n\nrequire github.com/anuy-lang/anuy " + anuyabiVersion + "\n"
	if root := os.Getenv("ANUY_REPLACE_ROOT"); root != "" {
		gomod += "\nreplace github.com/anuy-lang/anuy => " + root + "\n"
	}
	return os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644)
}

// inModule reports whether a go.mod governs dir or any parent — the
// module context the §6.4.2 Go-build invocation requires.
func inModule(dir string) bool {
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}

// packageName reads the package clause of generated text - always the
// first line (story 61).
func packageName(generated string) string {
	return strings.TrimPrefix(generated[:strings.IndexByte(generated, '\n')], "package ")
}

// reportPackageFailure renders the classified package check failure:
// a rendered diagnostic line (stdout) or a usage/internal message
// (stderr).
func reportPackageFailure(failure *packageFailure, cmd string, stdout, stderr io.Writer) int {
	if failure.diag != "" {
		fmt.Fprintln(stdout, failure.diag)
	} else {
		fmt.Fprintf(stderr, "anuy %s: %s\n", cmd, failure.msg)
	}
	return failure.code
}

// lowerPackageFiles lowers one package directory into the merged
// generated file (story 62).
func lowerPackageFiles(files []integration.SourceFile) (string, error) {
	lowerFiles := make([]lowering.File, len(files))
	for i, f := range files {
		lowerFiles[i] = lowering.File{Path: f.Path, Source: f.Source}
	}
	return lowering.LowerPackage(lowerFiles)
}

// runBuildDir implements `anuy build <dir>` (story 62, §6.4.1): the
// package checks as one unit and materializes ONE merged generated file
// (<package>.anuy.go, §6.2.6 RFC-010) before the §6.4.2 package-mode Go
// build.
func runBuildDir(dir string, stdout, stderr io.Writer) int {
	files, result, failure := analyzePackage(dir)
	if failure != nil {
		return reportPackageFailure(failure, "build", stdout, stderr)
	}
	if hasErrorSeverity(result.Diagnostics) {
		// Semantic errors: report instead of materializing.
		printPackageDiagnostics(stdout, files, result.Diagnostics)
		return exitDiag
	}

	// Environment pre-check happens before materialization (§6.12.2).
	if _, err := exec.LookPath("go"); err != nil {
		fmt.Fprintln(stderr, "anuy build: the Go toolchain is required:", err)
		return exitUsage
	}
	if !inModule(dir) {
		fmt.Fprintln(stderr, "anuy build: no go.mod found above", dir, "- anuy build runs inside a Go module (go mod init)")
		return exitUsage
	}

	generated, err := lowerPackageFiles(files)
	if err != nil {
		// A soundness-boundary reject is a user-facing check failure.
		fmt.Fprintf(stdout, "%s: error: %v\n", dir, err)
		return exitDiag
	}
	target := filepath.Join(dir, packageName(generated)+".anuy.go")
	if err := os.WriteFile(target, []byte(generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy build:", err)
		return exitUsage
	}

	cmd := exec.Command("go", "build", ".")
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return exitDiag
		}
		fmt.Fprintln(stderr, "anuy build:", err)
		return exitUsage
	}
	return exitOK
}

// runEmitGo implements `anuy emit-go` (§6.4.11–6.4.12): materialize the
// Go ABI view through the same lowering as build, to stdout or -o.
func runEmitGo(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("anuy emit-go", flag.ContinueOnError)
	fs.SetOutput(stderr)
	outFile := fs.String("o", "", "write the generated Go to this file instead of stdout")
	if err := fs.Parse(args); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: anuy emit-go [-o file] <file.anuy>")
		return exitUsage
	}
	target := fs.Arg(0)
	if isPackagePattern(target) {
		fmt.Fprintln(stderr, "anuy emit-go: package patterns are not supported - emit-go takes a single package")
		return exitUsage
	}
	if info, err := os.Stat(target); err == nil && info.IsDir() {
		return runEmitGoDir(target, *outFile, stdout, stderr)
	}
	path := target
	out := checkStages(path)
	if code, handled := out.stageFailure(path, stdout, stderr); handled {
		return code
	}
	if out.hasErrorDiag {
		printDiagnostics(stdout, path, out.source, out.result.Diagnostics)
		return exitDiag
	}

	if *outFile == "" {
		fmt.Fprint(stdout, out.generated)
		return exitOK
	}
	if err := os.WriteFile(*outFile, []byte(out.generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy emit-go:", err)
		return exitUsage
	}
	return exitOK
}

// runEmitGoDir implements `anuy emit-go <dir>` (story 65, §6.4.11): the
// merged Go ABI view of the package.
func runEmitGoDir(dir, outFile string, stdout, stderr io.Writer) int {
	files, result, failure := analyzePackage(dir)
	if failure != nil {
		return reportPackageFailure(failure, "emit-go", stdout, stderr)
	}
	if hasErrorSeverity(result.Diagnostics) {
		printPackageDiagnostics(stdout, files, result.Diagnostics)
		return exitDiag
	}
	generated, err := lowerPackageFiles(files)
	if err != nil {
		fmt.Fprintf(stdout, "%s: error: %v\n", dir, err)
		return exitDiag
	}
	if outFile == "" {
		fmt.Fprint(stdout, generated)
		return exitOK
	}
	if err := os.WriteFile(outFile, []byte(generated), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy emit-go:", err)
		return exitUsage
	}
	return exitOK
}

// runRun implements `anuy run` (§6.4.5–6.4.6): check, lower, shape the
// generated text into a main program and execute it with the local Go
// toolchain inside a temporary module. Program arguments after `--` are
// passed through (§6.4.6).
func runRun(args []string, stdout, stderr io.Writer) int {
	sep := -1
	for i, a := range args {
		if a == "--" {
			sep = i
			break
		}
	}
	fileArgs, progArgs := args, []string(nil)
	if sep >= 0 {
		fileArgs, progArgs = args[:sep], args[sep+1:]
	}
	fs := flag.NewFlagSet("anuy run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	if err := fs.Parse(fileArgs); err != nil {
		return exitUsage
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: anuy run [-- program args] <file.anuy>")
		return exitUsage
	}
	path := fs.Arg(0)
	if isPackagePattern(path) {
		fmt.Fprintln(stderr, "anuy run: package patterns are not supported - run takes a single program")
		return exitUsage
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return runRunDir(path, progArgs, stdout, stderr)
	}
	out := checkStages(path)
	if code, handled := out.stageFailure(path, stdout, stderr); handled {
		return code
	}
	if out.hasErrorDiag {
		printDiagnostics(stdout, path, out.source, out.result.Diagnostics)
		return exitDiag
	}
	return runGenerated(out.generated, progArgs, stdout, stderr)
}

// runRunDir implements `anuy run <dir>` (story 65, §6.4.5): the merged
// package program executes with the union imports.
func runRunDir(dir string, progArgs []string, stdout, stderr io.Writer) int {
	files, result, failure := analyzePackage(dir)
	if failure != nil {
		return reportPackageFailure(failure, "run", stdout, stderr)
	}
	if hasErrorSeverity(result.Diagnostics) {
		printPackageDiagnostics(stdout, files, result.Diagnostics)
		return exitDiag
	}
	generated, err := lowerPackageFiles(files)
	if err != nil {
		fmt.Fprintf(stdout, "%s: error: %v\n", dir, err)
		return exitDiag
	}
	return runGenerated(generated, progArgs, stdout, stderr)
}

// runGenerated shapes and executes the generated program in a temporary
// module (§6.4.5–6.4.6): the first generated line is the package clause
// (story 61), spliced into `package main` with the Run entry appended;
// program arguments pass through.
func runGenerated(generated string, progArgs []string, stdout, stderr io.Writer) int {
	dir, err := os.MkdirTemp("", "anuy-run-")
	if err != nil {
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}
	defer os.RemoveAll(dir)

	// Temporary module: the published anuyabi satisfies the generated
	// imports; ANUY_REPLACE_ROOT redirects to a repository checkout for
	// the development workflow.
	if err := writeTempModule(dir); err != nil {
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}
	// Main shaping of the deterministic generated text (story 47
	// determinism): the validation program is a package main whose entry
	// invokes the generated Run.
	program := "package main\n" + generated[strings.IndexByte(generated, '\n')+1:] + "\nfunc main() { Run() }\n"
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(program), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}

	cmd := exec.Command("go", append([]string{"run", "."}, progArgs...)...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if ok := asExitError(err, &exitErr); ok {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}
	return exitOK
}

func asExitError(err error, target **exec.ExitError) bool {
	if e, ok := err.(*exec.ExitError); ok {
		*target = e
		return true
	}
	return false
}

// printDiagnostics renders primary and related diagnostics in the
// §6.12.1 human format over the actual source coordinates.
func printDiagnostics(w io.Writer, path, source string, diags []semantic.Diagnostic) {
	for _, d := range diags {
		line, col := lineCol(source, d.Span.Start)
		message, _ := semantic.Message(d.Code)
		fmt.Fprintf(w, "%s:%d:%d: %s[%s]: %s\n", path, line, col,
			strings.ToLower(string(d.Severity)), d.Code, message)
		printDiagnostics(w, path, source, d.Related)
	}
}

func hasErrorSeverity(diags []semantic.Diagnostic) bool {
	for _, d := range diags {
		if string(d.Severity) == "Error" || hasErrorSeverity(d.Related) {
			return true
		}
	}
	return false
}

// lineCol converts a byte offset to a 1-based line and byte column —
// the byte model matches the §6.12.19 span convention.
func lineCol(source string, offset int) (int, int) {
	if offset > len(source) {
		offset = len(source)
	}
	line, col := 1, 1
	for i := 0; i < offset; i++ {
		if source[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}
	return line, col
}
