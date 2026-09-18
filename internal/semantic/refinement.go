// Evidence model from the kernel validation: early narrowing marks and
// the safe-navigation sketch (RFC-002 §6.4). Not wired into the
// flow-facts layer, which carries its own narrowing facts; kept as
// validation evidence.
package semantic

// Refinements marks the bindings proven non-nil at one point.
type Refinements map[BindingID]bool

// NewRefinements returns an empty refinement set.
func NewRefinements() Refinements { return make(Refinements) }

// MarkNonNil records the non-nil proof for a binding.
func (r Refinements) MarkNonNil(binding BindingID) { r[binding] = true }

// IsNonNil reports the recorded proof.
func (r Refinements) IsNonNil(binding BindingID) bool { return r[binding] }

// SafeNavigate evaluates member only for a present receiver.
func SafeNavigate(receiver func() Value, member func(Value) Value) Value {
	value := receiver()
	if value.IsNil {
		return Value{Type: NullableOf(value.Type), IsNil: true}
	}
	return Widen(member(value))
}
