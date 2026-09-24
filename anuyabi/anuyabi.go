// Package anuyabi is the ABI support package (RFC-009 §6.1.5–6.1.9):
// stable representation helpers for generated Go — no runtime, no
// scheduler, no hidden state. L2 graduation (ADR-0007, story 03-04):
// the contract is the Accepted RFC-009; the physical import path and
// release form are fixed by RFC-010 §6.7.6 (ADR-0014).
package anuyabi

import (
	"fmt"
	"reflect"
)

// Nullable is the canonical tagged nullable of the ABI v1 sketch
// (RFC-009 §6.1.7): Present == false is semantic nil (§6.1.8) and the
// zero value represents nil (§6.1.9). With one presence tag plus a value
// payload, every field combination is a valid state - there is no
// invalid tag combination (RFC-002 §6.9.6).
type Nullable[T any] struct {
	Value   T
	Present bool
}

// Some wraps a present value.
func Some[T any](value T) Nullable[T] {
	return Nullable[T]{Value: value, Present: true}
}

// None returns the nil carrier.
func None[T any]() Nullable[T] {
	return Nullable[T]{}
}

// Get reports the carried value and its presence.
func (n Nullable[T]) Get() (T, bool) {
	return n.Value, n.Present
}

// IsNil reports semantic nil.
func (n Nullable[T]) IsNil() bool {
	return !n.Present
}

// boundaryPrefix is the stable message prefix of the foreign boundary
// panic (RFC-009 §6.8.12): the wording is ABI, the carrier type is the
// recovery handle.
const boundaryPrefix = "anuy: invalid foreign value: "

// InvalidForeignValue is the canonical fail-stop carrier of a foreign
// entry violation (RFC-009 §6.8.3, §6.8.12): Param identifies the
// offending parameter or receiver, Reason the violated invariant. Hosts
// recover and classify through the type (errors.As), never by matching
// message text.
type InvalidForeignValue struct {
	Param  string
	Reason string
}

// Error renders the stable prefix plus the parameter and the invariant.
func (e *InvalidForeignValue) Error() string {
	return boundaryPrefix + e.Param + ": " + e.Reason
}

// Require panics with the canonical carrier - the single fail-stop exit
// of every boundary check (RFC-009 §6.8.3: panic before entering the
// native implementation).
func Require(param, reason string) {
	panic(&InvalidForeignValue{Param: param, Reason: reason})
}

// RequireEnum validates a contiguous ABI v1 discriminant (RFC-009
// §6.8.6): the valid range is 1..variants; zero stays invalid (§6.4.3).
func RequireEnum(param string, discriminant, variants uint32) {
	if discriminant == 0 || discriminant > variants {
		Require(param, fmt.Sprintf("enum discriminant %d outside 1..%d", discriminant, variants))
	}
}

// RequireNonNilInterface validates a non-null native interface entering
// from Go (RFC-009 §6.8.11): both the untyped nil interface and a
// typed-nil dynamic value (RFC-007 §6.3.7) fail the boundary. Nil-backed
// slices stay valid present values (RFC-007 §6.3.8) and never fail here.
func RequireNonNilInterface(param string, value any) {
	if value == nil {
		Require(param, "nil interface")
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer:
		if v.IsNil() {
			Require(param, "typed-nil "+v.Type().String())
		}
	}
}
