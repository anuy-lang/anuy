package parser

import (
	"testing"

	"github.com/anuy-lang/anuy/internal/semantic"
)

func TestParserErrorCarriesRegistryCodeAndSeverity(t *testing.T) {
	_, err := Parse("x := 1\n")
	perr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v, want *Error", err)
	}
	if perr.Category != ShortDeclaration || perr.Code != "ANUY1002" || perr.Severity != semantic.SeverityError {
		t.Fatalf("error = (%s, %s, %s), want (ShortDeclaration, ANUY1002, Error)", perr.Category, perr.Code, perr.Severity)
	}
}

func TestBlankIdentifierReadCarriesRegistryCode(t *testing.T) {
	_, err := Parse("_\n")
	perr, ok := err.(*Error)
	if !ok {
		t.Fatalf("err = %v, want *Error", err)
	}
	if perr.Category != BlankIdentifierRead || perr.Code != "ANUY1007" || perr.Severity != semantic.SeverityError {
		t.Fatalf("error = (%s, %s, %s), want (BlankIdentifierRead, ANUY1007, Error)", perr.Category, perr.Code, perr.Severity)
	}
}

// RFC-011 §6.2.15 (v3): the parser owns block 1xxx — every parser
// category must resolve to a code inside its own block (story-47: task 3).
func TestParserCategoriesStayInParserBlock(t *testing.T) {
	for _, category := range []ErrorCategory{
		UnsupportedSyntax,
		ShortDeclaration,
		BareDeclaration,
		TypedMultipleDeclaration,
		DuplicateAssignmentTarget,
		ArityMismatch,
		BlankIdentifierRead,
	} {
		desc, ok := semantic.DescriptorFor(semantic.DiagnosticCategory(category))
		if !ok {
			t.Fatalf("category %s is not registered", category)
		}
		if code := string(desc.Code()); code[:len("ANUY1")] != "ANUY1" {
			t.Fatalf("parser category %s carries %s, want block 1xxx", category, code)
		}
	}
}
