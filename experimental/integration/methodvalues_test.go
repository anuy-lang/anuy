package integration

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

// Story 72 (RFC-019 §6.20-6.21): method values and method expressions
// carry their resolved signature - the value drops the receiver, the
// expression takes it as the first parameter - and participate in the
// ordinary assignability checks.

const methodValueFixture = `type Counter struct {
n int
}
func (c Counter) Value() int {
return c.n
}
func Apply(transform func() int) int {
return transform()
}
var c = Counter{n: 0}
`

func TestMethodValueArgumentMatchClean(t *testing.T) {
	result, err := AnalyzeSource(methodValueFixture + "Apply(c.Value)\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestMethodValueArgumentMismatchReports(t *testing.T) {
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc (c Counter) Text(s string) int {\nreturn 1\n}\nfunc Apply(transform func(int) int) int {\nreturn transform(0)\n}\nvar c = Counter{n: 0}\nApply(c.Text)\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
	got := result.Diagnostics[0]
	if got.Code != "ANUY7008" || got.Severity != semantic.SeverityError {
		t.Fatalf("FunctionTypeMismatch = (%s, %s), want (ANUY7008, Error)", got.Code, got.Severity)
	}
}

func TestMethodExpressionArgumentMatchClean(t *testing.T) {
	result, err := AnalyzeSource(methodValueFixture + "func ApplyExpr(transform func(Counter) int) int {\nreturn transform(Counter{n: 1})\n}\nApplyExpr(Counter.Value)\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestMethodExpressionArgumentMismatchReports(t *testing.T) {
	// The expression's receiver is an explicit first parameter: a
	// zero-argument function type does not accept `Counter.Value`.
	result, err := AnalyzeSource(methodValueFixture + "Apply(Counter.Value)\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "FunctionTypeMismatch")
}

func TestPointerMethodExpressionArgumentMatchClean(t *testing.T) {
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc (c *Counter) Inc() {\nc.n = c.n + 1\n}\nfunc Apply(inc func(*Counter)) int {\nreturn 0\n}\nApply((*Counter).Inc)\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

func TestVariadicMethodValueMatchClean(t *testing.T) {
	// §6.14 x §6.20: the variadic method value keeps the ellipsis in its
	// signature.
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc (c Counter) Sum(values ...int) int {\nreturn 0\n}\nfunc Apply(sum func(...int) int) int {\nreturn sum(1)\n}\nvar c = Counter{n: 0}\nApply(c.Sum)\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("diagnostics = %v, want none", result.Diagnostics)
	}
}

// §6.20 v5: a safe segment guards a CALL, it does not form a method
// value - `c?.M` as a value rejects.
func TestSafeSegmentMethodValueReports(t *testing.T) {
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc (c Counter) Value() int {\nreturn c.n\n}\nfunc Find() Counter? {\nreturn Counter{n: 0}\n}\nvar c Counter? = Find()\nvar f = c?.Value\nf\n")
	if err != nil {
		t.Fatal(err)
	}
	assertSingleDiagnostic(t, result, "SafeSegmentMethodValue")
	got := result.Diagnostics[0]
	if got.Code != "ANUY4012" || got.Severity != semantic.SeverityError {
		t.Fatalf("SafeSegmentMethodValue = (%s, %s), want (ANUY4012, Error)", got.Code, got.Severity)
	}
}

func TestSafeSegmentFieldReadClean(t *testing.T) {
	// Safe field reads keep their existing conditional-presence
	// machinery - only method selectors reject.
	result, err := AnalyzeSource("type Counter struct {\nn int\n}\nfunc Find() Counter? {\nreturn Counter{n: 0}\n}\nvar c Counter? = Find()\nvar n int? = c?.n\nn\n")
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range result.Diagnostics {
		if d.Category == "SafeSegmentMethodValue" {
			t.Fatalf("safe field read reported: %v", d)
		}
	}
}
