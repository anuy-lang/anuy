package semantic

import "testing"

func TestChildScopeShadowsWithoutMutatingOuterBinding(t *testing.T) {
	outer := NewScope()
	_ = outer.Declare("x")
	inner := outer.Child()
	_ = inner.Declare("x")
	if outer.Resolve("x") == inner.Resolve("x") {
		t.Fatal("shadowed name must get distinct BindingID")
	}
	if err := inner.Assign("x"); err != nil {
		t.Fatal(err)
	}
	if err := outer.Assign("x"); err != nil {
		t.Fatal(err)
	}
}
