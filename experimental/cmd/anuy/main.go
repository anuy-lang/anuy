// Command anuy is the experimental CLI of the validation layer
// (ADR-0007 L0). Today it implements the `anuy check` slice of
// RFC-010 §6.4.7–6.4.9 — parse, semantic analyses and lowering
// validation of one .anuy file — with RFC-011 §6.12.1 human
// diagnostics, the §6.12.19 JSON document and the §6.12.20 exit codes.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
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
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: anuy <command> [flags] — commands: check")
		return exitUsage
	}
	switch args[0] {
	case "check":
		return runCheck(args[1:], stdout, stderr)
	default:
		fmt.Fprintf(stderr, "unknown command %q — commands: check\n", args[0])
		return exitUsage
	}
}

// runCheck implements the `anuy check` slice: parse + semantic
// analyses + lowering validation over one .anuy file (RFC-010
// §6.4.8 in the experimental-slice scope), rendered per RFC-011
// §6.12.1 or §6.12.19.
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
	source, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(stderr, "anuy check:", err)
		return exitUsage
	}
	text := string(source)

	result, aerr := integration.AnalyzeSource(text)
	if aerr != nil {
		// Parse errors carry registry codes — they are user-facing
		// diagnostics, not tool failures.
		if perr, ok := aerr.(*parser.Error); ok {
			line, col := lineCol(text, perr.Offset)
			fmt.Fprintf(stdout, "%s:%d:%d: %s[%s]: %s\n", path, line, col,
				strings.ToLower(string(perr.Severity)), perr.Code, perr.Message)
			return exitDiag
		}
		fmt.Fprintln(stderr, "anuy check:", aerr)
		return exitUsage
	}

	// Lowering validation belongs to check (§6.4.8/§6.4.9): a
	// soundness-boundary reject is a user-facing check failure.
	if _, lerr := lowering.Lower(text); lerr != nil {
		fmt.Fprintf(stdout, "%s: error: %v\n", path, lerr)
		return exitDiag
	}

	if *jsonOut {
		data, jerr := integration.DiagnosticsJSON(result)
		if jerr != nil {
			fmt.Fprintln(stderr, "anuy check:", jerr)
			return exitUsage
		}
		stdout.Write(data)
		stdout.Write([]byte("\n"))
	} else {
		printDiagnostics(stdout, path, text, result.Diagnostics)
	}

	if hasErrorSeverity(result.Diagnostics) {
		return exitDiag
	}
	return exitOK
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
