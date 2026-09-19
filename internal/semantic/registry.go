package semantic

// Code is the stable machine-readable diagnostic identifier, ANUY + four
// digits (RFC-011 §15). The block scheme and the concrete numbers are
// frozen by the story06 code-scheme proposal (owner decision 2026-09-16):
// codes are never renumbered or reused, and category renames do not
// change them.
type Code string

// Severity is the RFC-011 §16 diagnostic severity. The experimental layer
// uses Error for language validity and Warning for advisory lints;
// Information and Hint stay reserved for future owner decisions.
type Severity string

const (
	SeverityError   Severity = "Error"
	SeverityWarning Severity = "Warning"
)

// Descriptor is a registry row: one category paired with its code and
// severity. The fields are unexported, so a Descriptor can only come from
// the registry below — diagnostic constructors cannot be fed an
// unregistered code (CONTRACTS §1.2).
type Descriptor struct {
	category DiagnosticCategory
	code     Code
	severity Severity
}

func (d Descriptor) Category() DiagnosticCategory { return d.category }
func (d Descriptor) Code() Code                   { return d.code }
func (d Descriptor) Severity() Severity           { return d.severity }

func register(category DiagnosticCategory, code Code, severity Severity) Descriptor {
	return Descriptor{category: category, code: code, severity: severity}
}

// UncheckedError is the advisory lint category for a declared `error?`
// binding read nowhere (R1); the rule itself lands in 1-6-2-2.
const UncheckedError DiagnosticCategory = "UncheckedError"

// The registry — the single source of truth for category → (code,
// severity) (CONTRACTS §1.2), the approved proposal table verbatim.
var (
	// Parser block (1xxx): syntax validity.
	UnsupportedSyntaxDescriptor         = register("UnsupportedSyntax", "ANUY1001", SeverityError)
	ShortDeclarationDescriptor          = register("ShortDeclaration", "ANUY1002", SeverityError)
	BareDeclarationDescriptor           = register("BareDeclaration", "ANUY1003", SeverityError)
	TypedMultipleDeclarationDescriptor  = register("TypedMultipleDeclaration", "ANUY1004", SeverityError)
	DuplicateAssignmentTargetDescriptor = register("DuplicateAssignmentTarget", "ANUY1005", SeverityError)
	ArityMismatchDescriptor             = register("ArityMismatch", "ANUY1006", SeverityError)
	BlankIdentifierReadDescriptor       = register("BlankIdentifierRead", "ANUY1007", SeverityError)

	// Name resolution block (2xxx): lexical bindings.
	UnknownReadDescriptor            = register(UnknownRead, "ANUY2001", SeverityError) // D-01: error, not warning
	UnknownAssignmentDescriptor      = register(UnknownAssignment, "ANUY2002", SeverityError)
	SameScopeRedeclarationDescriptor = register(SameScopeRedeclaration, "ANUY2003", SeverityError)

	// Initialization dataflow block (3xxx): definite initialization.
	ReadBeforeInitializationDescriptor   = register(ReadBeforeInitialization, "ANUY3001", SeverityError)
	PackageInitializerRequiredDescriptor = register(PackageInitializerRequired, "ANUY3002", SeverityError)
	MissingReturnDescriptor              = register(MissingReturn, "ANUY3003", SeverityError) // D-6: result is a binding read by the caller (RFC-001 §13.3)

	// Nullability/narrowing block (4xxx).
	UnsafeMemberAccessDescriptor         = register(UnsafeMemberAccess, "ANUY4001", SeverityError)         // R2
	RedundantSafeNavigationDescriptor    = register(RedundantSafeNavigation, "ANUY4002", SeverityWarning)  // D-4, ADR-0005
	NullableArgumentDescriptor           = register(NullableArgument, "ANUY4003", SeverityError)           // D-3
	RedundantNilCheckDescriptor          = register(RedundantNilCheck, "ANUY4004", SeverityError)          // D-5
	NilToNonNullDescriptor               = register(NilToNonNull, "ANUY4005", SeverityError)               // D-1
	IncompleteConstructionDescriptor     = register(IncompleteConstruction, "ANUY4006", SeverityError)     // RFC-014 §6.3
	MissingEnumVariantDescriptor         = register(MissingEnumVariant, "ANUY4007", SeverityError)         // RFC-006 §6.3.4
	DuplicateMatchArmDescriptor          = register(DuplicateMatchArm, "ANUY4008", SeverityError)          // RFC-006 §6.3.5
	UnknownMatchVariantDescriptor        = register(UnknownMatchVariant, "ANUY4009", SeverityError)        // RFC-006 §6.1
	PropagationOutsideFallibleDescriptor = register(PropagationOutsideFallible, "ANUY6001", SeverityError) // RFC-005 §6.5.4
	NilArmOnNonNullEnumDescriptor        = register(NilArmOnNonNullEnum, "ANUY4010", SeverityError)        // RFC-006 §6.5.3

	// Lint block (5xxx): advisory analyzers.
	UncheckedErrorDescriptor = register(UncheckedError, "ANUY5001", SeverityWarning) // R1
)

// registry indexes the descriptors by category; the init-time checks keep
// the table honest (unique categories, unique codes).
var registry = func() map[DiagnosticCategory]Descriptor {
	descriptors := []Descriptor{
		UnsupportedSyntaxDescriptor,
		ShortDeclarationDescriptor,
		BareDeclarationDescriptor,
		TypedMultipleDeclarationDescriptor,
		DuplicateAssignmentTargetDescriptor,
		ArityMismatchDescriptor,
		BlankIdentifierReadDescriptor,
		UnknownReadDescriptor,
		UnknownAssignmentDescriptor,
		SameScopeRedeclarationDescriptor,
		ReadBeforeInitializationDescriptor,
		PackageInitializerRequiredDescriptor,
		MissingReturnDescriptor,
		UnsafeMemberAccessDescriptor,
		RedundantSafeNavigationDescriptor,
		NullableArgumentDescriptor,
		RedundantNilCheckDescriptor,
		NilToNonNullDescriptor,
		IncompleteConstructionDescriptor,
		MissingEnumVariantDescriptor,
		DuplicateMatchArmDescriptor,
		UnknownMatchVariantDescriptor,
		NilArmOnNonNullEnumDescriptor,
		PropagationOutsideFallibleDescriptor,
		UncheckedErrorDescriptor,
	}
	m := make(map[DiagnosticCategory]Descriptor, len(descriptors))
	codes := make(map[Code]DiagnosticCategory, len(descriptors))
	for _, d := range descriptors {
		if _, dup := m[d.category]; dup {
			panic("semantic: duplicate registry category " + d.category)
		}
		if owner, dup := codes[d.code]; dup {
			panic("semantic: code " + string(d.code) + " shared by " + string(owner) + " and " + string(d.category))
		}
		m[d.category] = d
		codes[d.code] = d.category
	}
	return m
}()

// DescriptorFor returns the registry descriptor for a category. false
// means the category is not registered — emitters treat that as a
// programming error, never as a diagnostic without a code.
func DescriptorFor(category DiagnosticCategory) (Descriptor, bool) {
	desc, ok := registry[category]
	return desc, ok
}

// catalog is the minimal canonical-English text per code (policy
// resolutions 1/3, 2026-09-15): the message is a rendering of last
// resort, never carried by kernel/integration diagnostics, and pinned by
// the golden test. The texts are the story06 proposal's references
// adapted to Anuy; final wording belongs to RFC-011.
var catalog = map[Code]string{
	"ANUY1001": "unsupported syntax",
	"ANUY1002": ":= is a syntax error",
	"ANUY1003": "declaration requires a type or an initializer",
	"ANUY1004": "typed multiple declaration is excluded from the confirmed grammar",
	"ANUY1005": "duplicate assignment target",
	"ANUY1006": "assignment arity mismatch",
	"ANUY1007": "_ does not hold a value",
	"ANUY2001": "read of an undefined binding",
	"ANUY2002": "assignment to an undefined binding",
	"ANUY2003": "redeclaration in the same scope",
	"ANUY3001": "read before initialization",
	"ANUY3002": "package-level binding requires an initializer",
	"ANUY3003": "missing return: declared result is not initialized on all paths",
	"ANUY4001": "only safe (?.) or non-null asserted calls are allowed on a nullable receiver",
	"ANUY4002": "redundant safe navigation: the receiver is known to be non-null",
	"ANUY4003": "nullable argument where a non-null is required",
	"ANUY4004": "redundant nil check: the value is non-null and cannot be nil",
	"ANUY4005": "nil used where a non-null value is required",
	"ANUY4006": "incomplete construction: every declared field must be initialized exactly once",
	"ANUY4007": "non-exhaustive switch over a native enum: not all variants are handled",
	"ANUY4008": "duplicate switch arm: the variant is already handled",
	"ANUY4009": "switch case is not a variant of the scrutinee's enum",
	"ANUY4010": "nil arm on a non-null enum scrutinee is unreachable",
	"ANUY6001": "propagation outside a fallible function",
	"ANUY5001": "error value is not checked",
}

// Message returns the canonical English text for a code (RFC-011 §14: the
// message is presentation; tests and evidence cite codes, not texts).
func Message(code Code) (string, bool) {
	text, ok := catalog[code]
	return text, ok
}
