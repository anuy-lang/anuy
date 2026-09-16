package semantic

import (
	"regexp"
	"testing"
)

// approvedScheme is the golden pin of the code-scheme proposal (owner
// decision 2026-09-16): one row per diagnostic category, never renumbered,
// never reused. Adding a category means adding a row here and a catalog
// entry in the same change.
var approvedScheme = []struct {
	category DiagnosticCategory
	code     Code
	severity Severity
}{
	{"UnsupportedSyntax", "ANUY1001", SeverityError},
	{"ShortDeclaration", "ANUY1002", SeverityError},
	{"BareDeclaration", "ANUY1003", SeverityError},
	{"TypedMultipleDeclaration", "ANUY1004", SeverityError},
	{"DuplicateAssignmentTarget", "ANUY1005", SeverityError},
	{"ArityMismatch", "ANUY1006", SeverityError},
	{"BlankIdentifierRead", "ANUY1007", SeverityError},
	{"UnknownRead", "ANUY2001", SeverityError},
	{"UnknownAssignment", "ANUY2002", SeverityError},
	{"SameScopeRedeclaration", "ANUY2003", SeverityError},
	{"ReadBeforeInitialization", "ANUY3001", SeverityError},
	{"PackageInitializerRequired", "ANUY3002", SeverityError},
	{"UnsafeMemberAccess", "ANUY4001", SeverityError},
	{"UncheckedError", "ANUY5001", SeverityWarning},
}

func TestRegistryMatchesApprovedCodeScheme(t *testing.T) {
	for _, want := range approvedScheme {
		desc, ok := DescriptorFor(want.category)
		if !ok {
			t.Fatalf("category %s is missing from the registry", want.category)
		}
		if desc.Code() != want.code || desc.Severity() != want.severity {
			t.Fatalf("%s = (%s, %s), want (%s, %s)", want.category, desc.Code(), desc.Severity(), want.code, want.severity)
		}
	}
	if len(registry) != len(approvedScheme) {
		t.Fatalf("registry holds %d categories, approved scheme pins %d", len(registry), len(approvedScheme))
	}
}

func TestRegistryCodesAreStableANUYQuads(t *testing.T) {
	pattern := regexp.MustCompile(`^ANUY\d{4}$`)
	seen := map[Code]DiagnosticCategory{}
	for category, desc := range registry {
		if !pattern.MatchString(string(desc.Code())) {
			t.Fatalf("%s carries code %q, want ANUY####", category, desc.Code())
		}
		if owner, dup := seen[desc.Code()]; dup {
			t.Fatalf("code %s is shared by %s and %s", desc.Code(), owner, category)
		}
		seen[desc.Code()] = category
	}
}

func TestCatalogCoversEveryRegisteredCode(t *testing.T) {
	for category, desc := range registry {
		text, ok := Message(desc.Code())
		if !ok {
			t.Fatalf("catalog has no entry for %s (%s)", category, desc.Code())
		}
		if text == "" {
			t.Fatalf("catalog text for %s (%s) is empty", category, desc.Code())
		}
	}
	if len(catalog) != len(registry) {
		t.Fatalf("catalog holds %d entries for %d registered codes", len(catalog), len(registry))
	}
}

func TestDiagnosticsCarryRegistryCodeAndSeverity(t *testing.T) {
	got := NewDiagnostic(UnknownReadDescriptor, 1, SourceSpan{File: "case.anuy", Start: 3, End: 4})
	if got.Code != "ANUY2001" || got.Severity != SeverityError {
		t.Fatalf("diagnostic = (%s, %s), want (ANUY2001, Error)", got.Code, got.Severity)
	}
	pkg := ValidatePackageBinding(false)
	if pkg.Code != "ANUY3002" || pkg.Severity != SeverityError {
		t.Fatalf("package diagnostic = (%s, %s), want (ANUY3002, Error)", pkg.Code, pkg.Severity)
	}
}
