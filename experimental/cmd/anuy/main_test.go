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
