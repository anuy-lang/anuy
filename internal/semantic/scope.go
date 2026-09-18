package semantic

// Scope is a lexical scope in the parent chain: it mints the binding IDs
// of its declarations and resolves reads through the chain.
type Scope struct {
	names  map[string]BindingID
	parent *Scope
	root   *Scope
	next   BindingID
}

// ScopeError is a scope-level reject; its category is a registry category.
type ScopeError struct{ Category DiagnosticCategory }

func (e *ScopeError) Error() string { return string(e.Category) }

const (
	SameScopeRedeclaration DiagnosticCategory = "SameScopeRedeclaration"
	UnknownAssignment      DiagnosticCategory = "UnknownAssignment"
	UnknownRead            DiagnosticCategory = "UnknownRead"
)

// NewScope returns the package root scope.
func NewScope() *Scope { s := &Scope{names: make(map[string]BindingID)}; s.root = s; return s }

// Child returns a nested scope whose declarations do not survive it.
func (s *Scope) Child() *Scope {
	return &Scope{names: make(map[string]BindingID), parent: s, root: s.root}
}
func (s *Scope) lookup(name string) (BindingID, bool) {
	if id, ok := s.names[name]; ok {
		return id, true
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return 0, false
}
func (s *Scope) Resolve(name string) BindingID {
	id, _ := s.lookup(name)
	return id
}

// Read reports UnknownRead when name resolves through no scope on the
// parent chain (D-01/P-25): a read without a binding is distinct from
// ReadBeforeInitialization, which requires a resolved binding.
func (s *Scope) Read(name string) *ScopeError {
	if _, ok := s.lookup(name); !ok {
		return &ScopeError{UnknownRead}
	}
	return nil
}

// Declare mints a binding in this scope; a duplicate name in the same
// scope rejects with SameScopeRedeclaration.
func (s *Scope) Declare(name string) *ScopeError {
	if _, exists := s.names[name]; exists {
		return &ScopeError{SameScopeRedeclaration}
	}
	s.root.next++
	s.names[name] = s.root.next
	return nil
}

// Assign checks that an assignment target resolves through the chain;
// unresolved targets reject with UnknownAssignment.
func (s *Scope) Assign(name string) *ScopeError {
	if _, exists := s.names[name]; !exists {
		if s.parent != nil {
			return s.parent.Assign(name)
		}
		return &ScopeError{UnknownAssignment}
	}
	return nil
}
