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
