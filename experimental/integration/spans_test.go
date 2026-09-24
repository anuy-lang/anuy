package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// RFC-011 §6.2.18: published diagnostics blame the source construct that
// triggered them. Kernel-emitted diagnostics carry the span of their
// operation, inside the enclosing statement's span (story-48: task 2).
func TestKernelDiagnosticsCarrySourceSpans(t *testing.T) {
	// Standalone UMA: the statement `user.save()` occupies 28..39.
	result, err := AnalyzeSource("var user User? = findUser()\nuser.save()\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != "UnsafeMemberAccess" {
		t.Fatalf("diagnostics = %#v, want one UnsafeMemberAccess", result.Diagnostics)
	}
	span := result.Diagnostics[0].Span
	if span.Start < 28 || span.End > 39 || span.Start >= span.End {
		t.Fatalf("UnsafeMemberAccess span = %d..%d, want inside 28..39", span.Start, span.End)
	}

	// Cascade: primary ReadBeforeInitialization + related UMA; the
	// statement `user.save()` occupies 32..43 — both spans land inside it.
	result, err = AnalyzeSource("var user User?\nvar f = func() {\nuser.save()\n}\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "ANUY3001" {
		t.Fatalf("diagnostics = %#v, want one ReadBeforeInitialization primary", result.Diagnostics)
	}
	primary := result.Diagnostics[0]
	// Story 50 (precision ladder): the receiver read blames the exact
	// identifier `user` (32..36), not the enclosing statement (32..43).
	if primary.Span != (semantic.SourceSpan{Start: 32, End: 36}) {
		t.Fatalf("primary span = %d..%d, want the receiver identifier 32..36", primary.Span.Start, primary.Span.End)
	}
	if len(primary.Related) != 1 {
		t.Fatalf("related = %#v, want one", primary.Related)
	}
	related := primary.Related[0].Span
	if related.Start < 32 || related.End > 43 || related.Start == 0 {
		t.Fatalf("related span = %d..%d, want inside 32..43", related.Start, related.End)
	}
}
