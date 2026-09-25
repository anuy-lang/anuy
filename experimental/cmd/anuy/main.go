// Command anuy is the experimental CLI of the validation layer
// (ADR-0007 L0). Implemented slices of RFC-010 §6.4: `anuy check`
// (§6.4.7–6.4.9), `anuy build` (§6.4.1–6.4.2: package-mode Go-build
// invocation over the materialized mixed package, §6.9.8 //line remap),
// `anuy emit-go` (§6.4.11–6.4.12) and `anuy run` (§6.4.5–6.4.6,
// temp-module). Diagnostics render per RFC-011 §6.12.1/§6.12.19 with
// the §6.12.20 exit codes.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
		fmt.Fprintln(stderr, "usage: anuy check [-json] <file.anuy>")
		return exitUsage
	}
	path := fs.Arg(0)
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
	return exitOK
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
	path := fs.Arg(0)
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
	out := checkStages(path)
	if code, handled := out.stageFailure(path, stdout, stderr); handled {
		return code
	}
	if out.hasErrorDiag {
		printDiagnostics(stdout, path, out.source, out.result.Diagnostics)
		return exitDiag
	}

	dir, err := os.MkdirTemp("", "anuy-run-")
	if err != nil {
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}
	defer os.RemoveAll(dir)

	// Temporary module: the published anuyabi satisfies the generated
	// imports; ANUY_REPLACE_ROOT redirects to a repository checkout for
	// the development workflow.
	gomod := "module runtmp\n\ngo 1.24\n\nrequire github.com/anuy-lang/anuy " + anuyabiVersion + "\n"
	if root := os.Getenv("ANUY_REPLACE_ROOT"); root != "" {
		gomod += "\nreplace github.com/anuy-lang/anuy => " + root + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		fmt.Fprintln(stderr, "anuy run:", err)
		return exitUsage
	}
	// Main shaping of the deterministic generated text (story 47
	// determinism): the validation program is a package main whose entry
	// invokes the generated Run. The declared package name (story 61)
	// is spliced out - the first generated line is always the package
	// clause.
	program := "package main\n" + out.generated[strings.IndexByte(out.generated, '\n')+1:] + "\nfunc main() { Run() }\n"
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
