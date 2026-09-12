package semantic

type Refinements map[BindingID]bool

func NewRefinements() Refinements                     { return make(Refinements) }
func (r Refinements) MarkNonNil(binding BindingID)    { r[binding] = true }
func (r Refinements) IsNonNil(binding BindingID) bool { return r[binding] }

// SafeNavigate evaluates member only for a present receiver.
func SafeNavigate(receiver func() Value, member func(Value) Value) Value {
	value := receiver()
	if value.IsNil {
		return Value{Type: NullableOf(value.Type), IsNil: true}
	}
	return Widen(member(value))
}
