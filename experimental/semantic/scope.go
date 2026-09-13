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
)

func NewScope() *Scope { s := &Scope{names: make(map[string]BindingID)}; s.root = s; return s }
func (s *Scope) Child() *Scope {
	return &Scope{names: make(map[string]BindingID), parent: s, root: s.root}
}
func (s *Scope) Resolve(name string) BindingID {
	if id, ok := s.names[name]; ok {
		return id
	}
	if s.parent != nil {
		return s.parent.Resolve(name)
	}
	return 0
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
