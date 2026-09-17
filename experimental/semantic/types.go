package semantic

// Nullability is the static nullability class of a binding in the typed
// slice (story 08, G1 decision): the declared/inferred layer beneath the
// flow facts. Unknown keeps platform semantics - dereference is free and
// nothing is established; Nullable requires the non-nil proof for ordinary
// member access; NonNull is the author's declaration contract (RFC-002 §6).
// The class is set once at the declaration site and never changes with the
// flow; flow-sensitive evidence stays in the narrowing facts (§22/§25/§26).
type Nullability uint8

const (
	// NullabilityUnknown carries no nullability evidence.
	NullabilityUnknown Nullability = iota
	// NullabilityNonNull is a declared or inferred non-null class.
	NullabilityNonNull
	// NullabilityNullable is a declared or inferred nullable class.
	NullabilityNullable
)
