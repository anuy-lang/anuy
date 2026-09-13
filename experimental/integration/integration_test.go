package integration

import "testing"

func TestAnalyzeSourceReportsReadBeforeInitialization(t *testing.T) {
	result, err := AnalyzeSource("var x int\nx\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Category != "ReadBeforeInitialization" {
		t.Fatalf("result = %#v", result)
	}
}
