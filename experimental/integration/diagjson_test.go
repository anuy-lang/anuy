package integration

import (
	"encoding/json"
	"testing"
)

// jsonDocument mirrors the RFC-011 §6.12.19 machine diagnostics shape:
// top-level entries carry code/severity/category/message/span, related
// entries carry code + span (story-47: task 2).
type jsonDocument struct {
	Version     int `json:"version"`
	Diagnostics []struct {
		Code     string `json:"code"`
		Severity string `json:"severity"`
		Category string `json:"category"`
		Message  string `json:"message"`
		Span     struct {
			File  string `json:"file"`
			Start int    `json:"start"`
			End   int    `json:"end"`
		} `json:"span"`
		Related []struct {
			Code string `json:"code"`
			Span struct {
				Start int `json:"start"`
				End   int `json:"end"`
			} `json:"span"`
		} `json:"related"`
	} `json:"diagnostics"`
}

// The cascade reproducer (CONTRACTS §2, decision 4) serializes as one
// primary with its suppressed deref nested as related, each carrying its
// own registry code.
func TestDiagnosticsJSONCascadeCarriesCodes(t *testing.T) {
	result, err := AnalyzeSource("var user User?\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	data, err := DiagnosticsJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	var doc jsonDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if doc.Version != 1 || len(doc.Diagnostics) != 1 {
		t.Fatalf("version=%d diagnostics=%d, want 1/1", doc.Version, len(doc.Diagnostics))
	}
	primary := doc.Diagnostics[0]
	if primary.Code != "ANUY3001" || primary.Category != "ReadBeforeInitialization" || primary.Severity != "Error" {
		t.Fatalf("primary = %s/%s/%s, want ANUY3001/ReadBeforeInitialization/Error", primary.Code, primary.Category, primary.Severity)
	}
	if primary.Message == "" {
		t.Fatal("primary message is empty: catalog rendering missing")
	}
	if len(primary.Related) != 1 || primary.Related[0].Code != "ANUY4001" {
		t.Fatalf("related = %#v, want one ANUY4001", primary.Related)
	}
}

// The advisory lint surface: severity survives serialization as registry
// data (RFC-011 §6.2.16), the message is catalog rendering.
func TestDiagnosticsJSONLintSeverity(t *testing.T) {
	result, err := AnalyzeSource("var err error? = f()\n")
	if err != nil {
		t.Fatal(err)
	}
	data, err := DiagnosticsJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	var doc jsonDocument
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, data)
	}
	if len(doc.Diagnostics) != 1 {
		t.Fatalf("diagnostics=%d, want one UncheckedError", len(doc.Diagnostics))
	}
	entry := doc.Diagnostics[0]
	if entry.Code != "ANUY5001" || entry.Severity != "Warning" || entry.Category != "UncheckedError" {
		t.Fatalf("lint = %s/%s/%s, want ANUY5001/Warning/UncheckedError", entry.Code, entry.Severity, entry.Category)
	}
	if entry.Message == "" {
		t.Fatal("lint message is empty: catalog rendering missing")
	}
}

// Determinism (§6.12.19): identical source yields byte-identical output.
func TestDiagnosticsJSONDeterministic(t *testing.T) {
	result, err := AnalyzeSource("var user User?\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	first, err := DiagnosticsJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := DiagnosticsJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Fatal("two serializations of the same result differ byte-wise")
	}
}
