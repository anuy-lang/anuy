package semantic

type Scope struct {
	names  map[string]BindingID
	parent *Scope
	root   *Scope
	next   BindingID
}

type ScopeError struct{ Category DiagnosticCategory }

func (e *ScopeError) Error() string { return string(e.Category) }

const (
	SameScopeRedeclaration DiagnosticCategory = "SameScopeRedeclaration"
	UnknownAssignment      DiagnosticCategory = "UnknownAssignment"
	UnknownRead            DiagnosticCategory = "UnknownRead"
)

func NewScope() *Scope { s := &Scope{names: make(map[string]BindingID)}; s.root = s; return s }
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
func (s *Scope) Declare(name string) *ScopeError {
	if _, exists := s.names[name]; exists {
		return &ScopeError{SameScopeRedeclaration}
	}
	s.root.next++
	s.names[name] = s.root.next
	return nil
}
func (s *Scope) Assign(name string) *ScopeError {
	if _, exists := s.names[name]; !exists {
		if s.parent != nil {
			return s.parent.Assign(name)
		}
		return &ScopeError{UnknownAssignment}
	}
	return nil
}
