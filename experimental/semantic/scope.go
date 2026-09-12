package semantic

type Scope struct {
	names map[string]BindingID
	next  BindingID
}

type ScopeError struct{ Category DiagnosticCategory }

func (e *ScopeError) Error() string { return string(e.Category) }

const (
	SameScopeRedeclaration DiagnosticCategory = "SameScopeRedeclaration"
	UnknownAssignment      DiagnosticCategory = "UnknownAssignment"
)

func NewScope() *Scope { return &Scope{names: make(map[string]BindingID)} }
func (s *Scope) Declare(name string) *ScopeError {
	if _, exists := s.names[name]; exists {
		return &ScopeError{SameScopeRedeclaration}
	}
	s.next++
	s.names[name] = s.next
	return nil
}
func (s *Scope) Assign(name string) *ScopeError {
	if _, exists := s.names[name]; !exists {
		return &ScopeError{UnknownAssignment}
	}
	return nil
}
