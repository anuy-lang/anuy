package semantic

import "testing"

func TestValidatePackageBindingRequiresInitializer(t *testing.T) {
	span := SourceSpan{File: "pkg.anuy", Start: 3, End: 9}
	if got := ValidatePackageBinding(false, span); got == nil || got.Category != PackageInitializerRequired {
		t.Fatalf("ValidatePackageBinding(false, span) = %#v, want PackageInitializerRequired", got)
	} else if got.Span != span {
		t.Fatalf("span = %v, want %v (RFC-011 §6.2.18)", got.Span, span)
	}
	if got := ValidatePackageBinding(true, span); got != nil {
		t.Fatalf("ValidatePackageBinding(true, span) = %#v, want nil", got)
	}
}

func TestUninitializedFactIsNotInitializedValue(t *testing.T) {
	facts := NewFactSet()
	if facts.State(BindingID(1)) != Uninitialized {
		t.Fatal("fresh storage must be semantically uninitialized")
	}
}
