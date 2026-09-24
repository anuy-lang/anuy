package integration

import (
	"encoding/json"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// The wire shapes are fixed by RFC-011 §6.12.19: top-level entries carry
// code/severity/category/message/span, related entries carry code + span.
// Struct order is the JSON order — serialization must stay deterministic
// and byte-identical for identical source (golden-pinned, story-47).
type spanJSON struct {
	File  string `json:"file"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

type relatedJSON struct {
	Code string   `json:"code"`
	Span spanJSON `json:"span"`
}

type diagnosticJSON struct {
	Code     string        `json:"code"`
	Severity string        `json:"severity"`
	Category string        `json:"category"`
	Message  string        `json:"message"`
	Span     spanJSON      `json:"span"`
	Related  []relatedJSON `json:"related"`
}

type diagnosticsDocumentJSON struct {
	Version     int              `json:"version"`
	Diagnostics []diagnosticJSON `json:"diagnostics"`
}

// DiagnosticsJSON renders the machine diagnostics document v1
// (RFC-011 §6.12.19). Kernel emission order is preserved (cascade
// deduplication already happened at emission, RFC-011 §6.2.13–6.2.14);
// message is catalog rendering (§6.2.17), never contract.
func DiagnosticsJSON(result Result) ([]byte, error) {
	doc := diagnosticsDocumentJSON{
		Version:     1,
		Diagnostics: make([]diagnosticJSON, 0, len(result.Diagnostics)),
	}
	for _, d := range result.Diagnostics {
		doc.Diagnostics = append(doc.Diagnostics, encodeDiagnostic(d))
	}
	return json.MarshalIndent(doc, "", "  ")
}

func encodeDiagnostic(d semantic.Diagnostic) diagnosticJSON {
	related := make([]relatedJSON, 0, len(d.Related))
	for _, r := range d.Related {
		related = append(related, relatedJSON{Code: string(r.Code), Span: encodeSpan(r.Span)})
	}
	message, _ := semantic.Message(d.Code)
	return diagnosticJSON{
		Code:     string(d.Code),
		Severity: string(d.Severity),
		Category: string(d.Category),
		Message:  message,
		Span:     encodeSpan(d.Span),
		Related:  related,
	}
}

func encodeSpan(s semantic.SourceSpan) spanJSON {
	return spanJSON{File: s.File, Start: s.Start, End: s.End}
}
