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
	{"MissingReturn", "ANUY3003", SeverityError},
	{"UnsafeMemberAccess", "ANUY4001", SeverityError},
	{"RedundantSafeNavigation", "ANUY4002", SeverityWarning},
	{"NullableArgument", "ANUY4003", SeverityError},
	{"RedundantNilCheck", "ANUY4004", SeverityError},
	{"NilToNonNull", "ANUY4005", SeverityError},
	{"IncompleteConstruction", "ANUY4006", SeverityError},
	{"MissingEnumVariant", "ANUY4007", SeverityError},
	{"DuplicateMatchArm", "ANUY4008", SeverityError},
	{"UnknownMatchVariant", "ANUY4009", SeverityError},
	{"NilArmOnNonNullEnum", "ANUY4010", SeverityError},
	{"PropagationOutsideFallible", "ANUY6001", SeverityError},
	// Story 35 (RFC-005 §8.1): the strict fallible diagnostics block.
	{"InvalidFailureReturn", "ANUY6002", SeverityError},
	{"InvalidTry", "ANUY6003", SeverityError},
	{"MixedReturn", "ANUY6004", SeverityError},
	// Story 36 (RFC-005 §8.1.4): conditional correlation.
	{"UnavailableSuccessResult", "ANUY6005", SeverityError},
	// Story 37 (RFC-005 §8.1.1): must-consume.
	{"IgnoredError", "ANUY6006", SeverityError},
	// Story 39 (RFC-004): interfaces and explicit impl.
	{"InterfaceMethodMissing", "ANUY7001", SeverityError},
	{"InterfaceMethodMismatch", "ANUY7002", SeverityError},
	{"DuplicateImpl", "ANUY7003", SeverityError},
	{"ImplUnknownInterface", "ANUY7004", SeverityError},
	// Story 40 (RFC-004 §6.4/§8.1.4-8.1.5): conversions and dispatch.
	{"UndefinedInterfaceMember", "ANUY7005", SeverityError},
	{"MissingExplicitConformance", "ANUY7006", SeverityError},
	// Story 41 (RFC-007 §8.2): unsafe core.
	{"UnsafeOperationOutside", "ANUY8001", SeverityError},
	{"UnsafeCallOutsideContext", "ANUY8002", SeverityError},
	{"RedundantUnsafeAssertion", "ANUY5002", SeverityWarning},
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

// RFC-011 §6.2.16 (v3): severity follows the diagnostic class, not the
// block — the block is the area. Advisory codes outside the lint block
// 5xxx must be recorded in advisoryExceptions (and in §6.2.16); block
// 9xxx stays reserved for backend/tooling-consistency diagnostics
// (story-47: task 3).
var advisoryExceptions = map[Code]string{
	"ANUY4002": "RedundantSafeNavigation", // D-4, ADR-0005
}

func TestRegistrySeverityFollowsCodeBlock(t *testing.T) {
	for category, desc := range registry {
		block := desc.Code()[len("ANUY")] - '0'
		if block == 0 || block == 9 {
			t.Fatalf("%s uses reserved block %c (RFC-011 §6.2.15)", category, desc.Code()[len("ANUY")])
		}
		switch desc.Severity() {
		case SeverityWarning:
			if block == 5 {
				continue // advisory block defaults to Warning
			}
			if owner, ok := advisoryExceptions[desc.Code()]; !ok || owner != string(category) {
				t.Fatalf("%s (%s) = Warning outside advisory block 5xxx; record the advisory in RFC-011 §6.2.16 and advisoryExceptions", category, desc.Code())
			}
		default:
			if block == 5 {
				t.Fatalf("%s (%s) = %s in advisory block 5xxx", category, desc.Code(), desc.Severity())
			}
		}
	}
}
