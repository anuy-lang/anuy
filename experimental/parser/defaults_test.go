package parser

import (
	"errors"
	"testing"
)

// Story 77 (RFC-019 §6.4/§8.15): parameters carry no default values - a
// targeted reject, not the generic type error.
func TestDefaultParameterRejects(t *testing.T) {
	_, err := Parse("package p\nfunc Connect(timeout int = 30) {\n}\n")
	var perr *Error
	if !errors.As(err, &perr) || perr.Code != "ANUY1001" {
		t.Fatalf("err = %v, want ANUY1001", err)
	}
}
