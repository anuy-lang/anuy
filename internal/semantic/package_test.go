package semantic

import "testing"

func TestValidatePackageBindingRequiresInitializer(t *testing.T) {
	if got := ValidatePackageBinding(false); got == nil || got.Category != PackageInitializerRequired {
		t.Fatalf("ValidatePackageBinding(false) = %#v, want PackageInitializerRequired", got)
	}
	if got := ValidatePackageBinding(true); got != nil {
		t.Fatalf("ValidatePackageBinding(true) = %#v, want nil", got)
	}
}

func TestUninitializedFactIsNotInitializedValue(t *testing.T) {
	facts := NewFactSet()
	if facts.State(BindingID(1)) != Uninitialized {
		t.Fatal("fresh storage must be semantically uninitialized")
	}
}
