# RFC-007 — Unsafe and Foreign Contracts

**Status:** Accepted
**RFC:** 007
**Title:** Unsafe and Foreign Contracts
**Language:** Anuy
**Area:** Safety / Trust / Foreign Boundaries / Unsafe
**Version:** 4
**Date:** 2026-09-24
**Requires:** RFC-000, RFC-001, RFC-002, RFC-003, RFC-004, RFC-005, RFC-006, RFC-008, RFC-009, RFC-011
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-007 reviewed against RFC-000 (safety through contracts, not through the implementation language), RFC-001 (uninitialized is inaccessible even to unsafe), RFC-002 (foreign ≠ nullable), RFC-004 (unsafe does not create conformance), and RFC-006 (foreign discriminants). Delegations: contract syntax and the Go mapping — RFC-008; representation-level intrinsics — RFC-009 (Section 7).

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

2026-09-24: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 3 → 4.

---

## 1. Abstract

Anuy distinguishes two reasons for the absence of a proof: the value came from an environment where invariants are not guaranteed (**foreign boundary**), and the programmer knows a fact that the compiler cannot prove (**unsafe assertion**). It follows that calling Go is not inherently unsafe: a safe call is possible when the boundary contract preserves the invariants. `unsafe { ... }` is a lexical privileged context for specific proof escapes that does not disable checking; `unsafe func` is a contract of caller-side preconditions; trusted foreign contracts are explicit, local, and auditable.

## 2. Solutions

> **`unsafe` is required when the programmer takes responsibility for an invariant the compiler cannot prove.**

> **Calling Go is not inherently unsafe.**

> **A foreign value may enter safe Anuy as type `T` only when the boundary can justify every invariant of `T`.**

> **`unsafe` grants access to specific privileged operations; it does not disable Anuy checking.**

> **Safe Anuy may rely on an invariant after foreign mutation only if the boundary contract guarantees that the mutation preserves that invariant, or the value is validated again.**

> **Safe Anuy trusts proofs and checks; `unsafe` is the explicit place where a programmer chooses to become the proof.**

> **Foreign code is not unsafe by definition. Unproven invariants are.**

## 3. Motivation

A language on top of Go must explain how much Go code can be trusted. The naive answer — "all of Go is unsafe" — would make the ecosystem unusable; the opposite — "Go is safe by default" — would destroy the stronger invariants of Anuy (non-null, closed enums, initialized aggregates). RFC-007 defines a precise model: trust is decided at the boundary by a contract (structural / checked / trusted), while `unsafe` is reserved for the places where the programmer consciously becomes the proof.

## 4. Goals

The following goals follow implicitly from the core model: Go calls MAY be fully safe; foreign uncertainty is never silently strengthened; unsafe sites are explicit, small, searchable, and documentable; compiler assumptions after proof/validation/assertion remain valid; all invariants are preserved at the foreign boundary, including mutation and callbacks.

## 5. Non-goals

This RFC does not define:

- pervasive `foreign T` taint system;
- universal `ForeignValidationError`;
- contract declaration syntax (RFC-008);
- representation-level intrinsic syntax (RFC-009);
- concrete Go interop mapping (RFC-008);
- automatic provability of prose safety documentation.

## 6. Specification

### 6.1 Core Model

#### 6.1.1 Core model

Anuy distinguishes three ways to establish an invariant.

**Static proof** — the compiler proves the invariant.

Example:

```anuy
if user == nil {
    return
}

user.save()
```

**Runtime validation** — the program checks the value and only after a successful check creates the stronger native value.

**Unsafe assertion** — the programmer asserts the invariant without a check:

```anuy
unsafe {
    var user = assume_non_nil(maybeUser)
}
```

Afterwards the compiler is entitled to treat the assertion as true.

Responsibility for its truth lies with the unsafe code.

### 6.2 Native and Foreign Values

#### 6.2.1 Native value

**Native value** — a value for which Anuy is entitled to treat all invariants of its source type as established.

For example, an initialized:

```text
User
```

guarantees the invariants of `User`.

An initialized:

```text
Color
```

guarantees:

```text
value ∈ declared variants of Color
```

An initialized:

```text
*File
```

guarantees non-nullness.

A native value does not have to be physically created by Anuy code.

A foreign value, after successful validation, can also become a native value.

#### 6.2.2 Foreign value

**Foreign value** — a value whose origin lies outside the proof domain of safe Anuy.

Typical sources:

- imported Go code;
- `.go` files in a mixed package;
- reflection;
- generated external code;
- raw memory;
- `unsafe.Pointer`;
- manually trusted interop contracts;
- callbacks invoked from Go;
- values previously exposed to uncontrolled foreign mutation.

`foreign` is a semantic provenance concept.

RFC-007 does not introduce a mandatory source type:

```text
foreign T
```

for each such value.

#### 6.2.3 Foreign does not mean invalid

A foreign value is not necessarily bad or malformed.

For example, the Go function:

```go
func Count() int
```

can safely return the ordinary Anuy type:

```text
int
```

because the Go `int` representation violates none of the additional Anuy invariants for `int`.

The foreign boundary matters only where the Anuy source type promises more than the foreign representation guarantees.

#### 6.2.4 Stronger Anuy invariants

Examples of invariants that the foreign Go representation may not guarantee:

```text
non-null pointer
closed enum discriminant
fully initialized aggregate
strict fallible-result correlation
absence of typed-nil dynamic values
validated wrapper state
representation-specific tagged nullable state
native interface/conformance assumptions
```

The interop layer MUST preserve these guarantees.

### 6.3 Conservative Boundary and Examples

#### 6.3.1 Conservative boundary rule

Foreign uncertainty MUST NEVER be silently strengthened.

The main rule:

> **A foreign value may enter safe Anuy as type `T` only when the boundary can justify every invariant of `T`.**

Justification can be:

1. guaranteed foreign representation;
2. compiler proof;
3. runtime validation;
4. explicit trusted/unsafe contract.

Without one of these grounds, projection to the stronger Anuy type is forbidden.

#### 6.3.2 Example: nullable pointer

Go function:

```go
func Lookup() *User
```

can return `nil`.

Therefore, without an additional contract, its natural safe Anuy projection is:

```anuy
func Lookup() *User?
```

and not:

```anuy
func Lookup() *User
```

Go by itself provides no non-null guarantee.

#### 6.3.3 Validating foreign pointer

The programmer can obtain a native non-null value through ordinary safe control flow:

```anuy
var user = goapi.Lookup()

if user == nil {
    return error ErrUserMissing
}

use(user)
```

No `unsafe` is required.

The compiler proved the invariant.

#### 6.3.4 Trusted non-null contract

Sometimes an external API documents:

> this function never returns nil.

If the Go type system does not express this, the Anuy compiler cannot obtain the guarantee automatically.

The interop layer has two options:

```text
validate at runtime
```

or:

```text
explicitly trust the external contract
```

The second option is an unsafe trust assertion and MUST be auditable.

The exact contract syntax is defined by RFC-008.

#### 6.3.5 Example: enum

Native:

```anuy
enum Color {
    Red
    Green
    Blue
}
```

guarantees a closed set.

A foreign Go integer:

```go
type Color uint32
```

can contain:

```text
1000
```

Therefore an arbitrary Go `Color` MUST NOT be silently treated as the native Anuy `Color`.

The boundary MUST:

- check the discriminant;
- or have a trusted contract that proves validity.

#### 6.3.6 Example: zeroed aggregate

An Anuy aggregate can have the semantic invariant:

```text
all fields initialized
```

Go code is capable of creating a zeroed backing representation.

Therefore the existence of a compatible Go struct representation does not yet prove that the value is a valid native Anuy object.

Representation compatibility:

```text
≠
```

semantic validity.

#### 6.3.7 Example: interfaces

A Go interface value can contain a typed nil:

```go
var p *File = nil
var r io.Reader = p
```

where:

```text
r != nil
```

at the Go interface level.

If Anuy non-null interface semantics requires a valid non-null dynamic receiver, the boundary MUST account for this situation.

Foreign interface values MUST NOT automatically be treated as satisfying the stronger native invariant based only on the outer Go interface bit pattern.

The exact normalization/validation is defined by RFC-008.

#### 6.3.8 Slices are type-specific

A foreign nil-backed Go slice does not necessarily violate an Anuy invariant.

RFC-002 admits an ordinary:

```text
[]T
```

as a present slice value with a nil Go backing.

Consequently:

```text
nil backing
```

by itself does not mean semantic `nil`.

Boundary validation depends on the actual Anuy source semantics of the specific type.

### 6.4 Foreign Contracts

#### 6.4.1 No pervasive taint system

RFC-007 does not introduce pervasive source-level taint propagation:

```text
foreign int
foreign string
foreign User
```

throughout the program.

Instead, trust is concentrated at the boundary.

Once a value is:

- inherently representation-safe;
- or validated;
- or admitted through an explicit unsafe assertion,

it is used as an ordinary native value.

This keeps the language simple.

#### 6.4.2 Foreign contracts

The interop boundary is described by a **foreign contract**.

Conceptually, the contract connects:

```text
foreign ABI
    ↓
Anuy semantic signature
```

The contract can carry information about:

- nullability;
- result semantics;
- enum validity;
- initialization;
- interface guarantees;
- mutation;
- alias retention;
- callbacks;
- panic behavior where relevant;
- validation requirements;
- safe/unsafe callability.

The exact syntax and metadata format belong to RFC-008.

#### 6.4.3 Contract classes

RFC-007 defines three semantic classes of foreign guarantees.

**Structural** — the guarantee is derived directly from the foreign ABI/type system.

For example, a compatible integer value.

No additional trust is involved.

**Checked** — the boundary performs runtime validation.

After successful validation the value becomes native.

**Trusted** — the guarantee cannot be checked, or the compiler intentionally does not check it.

The interop declaration takes responsibility.

Such trust MUST be explicit and auditable.

### 6.5 Foreign Calls

#### 6.5.1 Safe foreign call

A foreign call is safe if, for all inputs and outputs, the Anuy boundary is able to preserve the source invariants.

For example:

```go
func Add(a int, b int) int
```

can be imported as fully safe.

There is no reason to write:

```anuy
unsafe {
    goapi.Add(1, 2)
}
```

only because the implementation is written in Go.

#### 6.5.2 Unsafe foreign call

A foreign operation requires an unsafe context if its correctness depends on a caller obligation that the Anuy type system does not express and the boundary does not check.

Conceptual examples:

```text
pointer must refer to N bytes
address must remain valid until callback returns
buffer must not be mutated concurrently
integer must correspond to valid discriminant
caller guarantees returned pointer is non-null
```

In such cases unsafe is needed because of the invariant, not because of the implementation language.

### 6.6 Unsafe Block and Assertions

#### 6.6.1 `unsafe` block

V1 introduces the lexical syntax:

```anuy
unsafe {
    ...
}
```

Unsafe-only operations can be executed only inside such a block.

Example:

```anuy
var user User? = ...

unsafe {
    var definiteUser = assume_non_nil(user)
    use(definiteUser)
}
```

#### 6.6.2 Unsafe block is lexical

`unsafe` refers to a source region.

It does not create a separate runtime mode.

It does not:

- set a flag;
- disable runtime protection;
- create a frame;
- change the calling convention.

It is a compile-time permission context.

#### 6.6.3 Unsafe does not disable the compiler

Critical rule:

> **`unsafe` grants access to specific privileged operations; it does not disable Anuy checking.**

Inside an unsafe block the following still apply:

- name resolution;
- type checking;
- definite initialization;
- scope rules;
- assignment rules;
- error handling rules;
- exhaustive matching;
- method-set rules;
- generic constraints.

For example:

```anuy
unsafe {
    var x int
    print(x)
}
```

remains a compile error.

#### 6.6.4 No reading uninitialized storage

Unsafe does not provide:

```text
assume_initialized(x)
```

and does not allow reading the backing zero value of an uninitialized binding.

```anuy
var user User

unsafe {
    use(user) // still error
}
```

The reason is fundamental:

> uninitialized is compiler state, not a hidden runtime value available to unsafe code.

The compiler is not even required to materialize meaningful backing storage before initialization.

#### 6.6.5 Unsafe assertion

An unsafe intrinsic can assert a specific invariant.

Canonical v1 example:

```anuy
unsafe {
    var user = assume_non_nil(maybeUser)
}
```

If:

```text
maybeUser : T?
```

then:

```text
assume_non_nil(maybeUser) : T
```

#### 6.6.6 `assume_non_nil`

`assume_non_nil` performs no runtime check.

It is a promise:

```text
operand != nil
```

The compiler is entitled to use this fact.

Unlike:

```anuy
if value == nil {
    ...
}
```

it is not a validation.

#### 6.6.7 Failed unsafe assumption

If the programmer writes:

```anuy
unsafe {
    var user = assume_non_nil(nilUser)
}
```

and the assumption is false, the execution violates the unsafe contract.

After that, the Anuy safety guarantees no longer apply to this execution.

Depending on the lowering, the violation can lead to:

- panic;
- invalid foreign call;
- corrupted semantic state;
- backend-dependent behavior.

The compiler is not required to insert a defensive check.

#### 6.6.8 Prefer proof over unsafe

If the compiler is able to prove the invariant:

```anuy
if user != nil {
    use(user)
}
```

`unsafe` is not needed.

Tooling MAY warn about a redundant unsafe assertion.

Unsafe is not intended as an ergonomic shortcut for ordinary control flow.

#### 6.6.9 Unsafe values may escape

A value created through an unsafe assertion can escape the unsafe block:

```anuy
func requireUser(value User?) User {
    unsafe {
        return assume_non_nil(value)
    }
}
```

Upon return the caller receives an ordinary `User`.

This is intentional.

The unsafe abstraction exists precisely to implement safe-looking higher-level APIs.

But the correctness of such a function depends entirely on the validity of its internal unsafe assumption.

#### 6.6.10 Safe wrapper principle

A function can use unsafe internally and remain safe for the caller:

```anuy
func validatedOperation(...) T {
    ...
    unsafe {
        ...
    }
}
```

if its public contract does not require the caller to uphold unprovable preconditions.

This allows unsafe implementation details to stay localized.

#### 6.6.11 The assertion establishes a flow fact about its operand

Design note, 2026-09-22 (slice design): §6.6.5 records the result of the assertion (`var definiteUser = assume_non_nil(user)`), but not the fate of the operand itself. The assertion is a promise about `operand != nil`, so the fact is attached to the operand.

`assume_non_nil(x)` establishes a non-null flow fact about `x` at the point of the assertion; from then on the fact lives by the ordinary flow rules (RFC-002 narrowing):

1. the var form `var d = assume_non_nil(x)` and the statement form `assume_non_nil(x)` are both valid;
2. after the assertion the compiler is entitled to treat `x` as non-null until invalidation by an ordinary assignment (RFC-003 §26);
3. an assertion over an operand of unknown class (foreign tolerance) is valid — this is its main foreign use case;
4. an assertion over an already-proven non-null operand is redundant — §6.6.8, advisory §8.2.6.

#### 6.6.12 Nested unsafe blocks

Unsafe blocks MAY nest. Nesting adds no capabilities: an inner block inside an already-unsafe context is redundant, but is not an error. Every privileged operation requires at least one surrounding unsafe block (§6.6.1); more than one is allowed.

#### 6.6.13 Intrinsic namespace is reserved

Design note, 2026-09-22 (slice design, discussion with the owner): `assume_non_nil` is not a function but a compiler intrinsic in call form. The kernel recognizes it by name, so shadowing it with a user declaration would silently replace the trust operation with an ordinary call and punch a hole in the safety theorem (§6.12.5).

The unsafe-core compiler intrinsics are reserved as names:

```text
assume_non_nil
```

(the list is extended by RFCs as new intrinsics are added). A declaration (`func`, binding, parameter, method, type) with a reserved intrinsic name is a compile error. Project precedent: RFC-013 §6.4.3 reserves built-in names in attribute position (`must_use`, `deprecated`).

The separation from attributes (RFC-013): **attributes are contracts of declarations** (`#[pure]`, the future trusted foreign contracts of RFC-008 OQ-1 — declaration, metadata, audit); **intrinsics are pointwise flow facts** (an assertion at a specific point of control flow inside an unsafe context). RFC-013 §6.1.6 (no statement/expression targets) makes attributes unsuitable for the latter; trust in another declaration (§6.10.2) is unsuitable for the former.

### 6.7 Unsafe Functions

#### 6.7.1 Unsafe function

Sometimes the obligation belongs to the caller.

For this, v1 provides:

```anuy
unsafe func ...
```

Conceptually:

```anuy
unsafe func fromRawAddress(...) *Buffer {
    ...
}
```

The call requires an unsafe context:

```anuy
unsafe {
    var buffer = fromRawAddress(...)
}
```

#### 6.7.2 Meaning of `unsafe func`

`unsafe func` means:

> Correct use of this function requires preconditions not fully represented or checked by its type signature.

This is part of the public contract.

The caller accepts responsibility when calling.

#### 6.7.3 Unsafe function body remains checked

The declaration:

```anuy
unsafe func operation(...) T {
    ...
}
```

does not turn the whole body into an implicit unsafe block.

A privileged operation must still be locally marked:

```anuy
unsafe func operation(...) T {
    ...

    unsafe {
        ...
    }
}
```

This makes unsafe implementation sites auditable separately from the unsafe call contract.

#### 6.7.4 Why both constructs exist

```text
unsafe block
```

says:

> here the implementation makes an unprovable claim.

```text
unsafe func
```

says:

> the caller is obliged to provide an unprovable invariant.

These are different responsibilities.

#### 6.7.5 Unsafe API documentation

Every exported `unsafe func` MUST have machine/tooling-visible unsafe contract documentation.

At minimum it must be possible to describe:

```text
Safety:
    caller must guarantee ...
```

The exact documentation syntax can be an ordinary doc comment convention/tooling rule.

The compiler is not required to prove the prose condition automatically.

### 6.8 Unsafe and Language Boundaries

#### 6.8.1 Unchecked conversions

RFC-007 allows privileged unchecked conversion operations to exist.

However, `unsafe` does not mean arbitrary bit reinterpretation of any `A` into any `B`.

The compiler still checks structural legality.

For example, an unsafe conversion MAY require:

- known compatible representation;
- compatible size/alignment;
- supported backend mapping;
- appropriate pointer category.

The exact intrinsic syntax is defined by RFC-008/RFC-009.

#### 6.8.2 Unsafe cannot invent ABI

Unsafe MUST NOT be used as:

```text
pretend these unrelated representations have the same ABI
```

if the backend is not able to perform the conversion correctly.

Unsafe removes the semantic proof obligation.

It does not make an impossible machine-representation operation possible.

#### 6.8.3 Unsafe and interface conformance

Unsafe is not a shortcut around RFC-004:

```text
missing impl
```

does not automatically turn into a valid conformance inside an unsafe block.

That is, the ordinary:

```anuy
unsafe {
    var reader Reader = value
}
```

still requires a valid interface relation.

Foreign/adaptor mechanisms MUST use the rules of RFC-004/RFC-008.

`unsafe` does not create methods and does not change the Go method set.

#### 6.8.4 Unsafe and errors

Unsafe does not allow silent error discard.

```anuy
unsafe {
    file.Close() // still error if Close returns error?
}
```

The programmer must still:

```anuy
try file.Close()
```

handle the error, or:

```anuy
discard file.Close()
```

to explicitly ignore it.

#### 6.8.5 Unsafe and enums

Unchecked creation of a native enum value is allowed only as an explicit unsafe operation with the precondition:

```text
raw discriminant must correspond to a declared variant
```

After the assertion the compiler is entitled to treat the enum as valid.

Violating this precondition is an unsafe contract violation.

Safe code never creates an invalid discriminant.

#### 6.8.6 Unsafe and nullability

The primary nullability escape hatch:

```anuy
unsafe {
    var x = assume_non_nil(value)
}
```

V1 does not introduce the postfix:

```text
value!
value!!
```

This keeps the unsafe assertion visually heavy and easily searchable.

#### 6.8.7 Unsafe and raw representation

Operations that observe or create representation outside source semantics are privileged.

For example, the conceptual categories:

```text
raw representation access
pointer reinterpretation
manual discriminant construction
representation-compatible unchecked cast
```

The exact low-level API is defined by RFC-009.

#### 6.8.8 `unsafe.Pointer`

Interop with Go `unsafe.Pointer` belongs to the privileged area.

The type itself can appear in foreign signatures, but operations that turn a raw pointer into a typed native value or back, based on programmer assumptions, require an unsafe context.

Anuy does not attempt to make Go raw-pointer programming safe automatically.

### 6.9 Validation, Mutation and Callbacks

#### 6.9.1 Validation is safe

A runtime check is not unsafe by itself.

For example, conceptually:

```text
foreign discriminant
    ↓
check against known enum variants
    ↓
native enum
```

is a safe operation.

Unsafe is needed only if the programmer wants to skip the proof/check.

#### 6.9.2 No universal validation failure mechanism

Not every foreign validation can automatically conclude in the same way.

Depending on the API, the failure can naturally be:

- `error?`;
- `T?`;
- explicit validation result;
- rejected callback;
- impossible to expose safely.

Therefore RFC-007 does not introduce a universal:

```text
ForeignValidationError
```

The exact projection policy is defined by RFC-008.

#### 6.9.3 Safe projection requires a failure path

If boundary validation can fail, the safe API MUST have a way to express that failure.

If the original ABI provides no such channel and the wrapper cannot add one without changing the API surface, the compiler cannot pretend the validation is infallible.

In that case the following are allowed:

```text
different safe wrapper API
unsafe/trusted contract
keep value in weaker foreign form
```

#### 6.9.4 Foreign mutation

A single validation is not enough if foreign code retains the ability to change the representation afterwards.

Example:

```text
validate native enum field
↓
give mutable pointer to Go
↓
Go writes invalid discriminant
↓
Anuy reads field
```

Such a sequence would violate the enum invariant.

#### 6.9.5 Mutation preservation rule

The main rule:

> **Safe Anuy may rely on an invariant after foreign mutation only if the boundary contract guarantees that the mutation preserves that invariant, or the value is validated again.**

This concerns:

- enum discriminants;
- non-null fields;
- tagged nullable representation;
- aggregate validity;
- any future refined native type.

#### 6.9.6 Retained aliases

Especially dangerous is foreign code that retains a pointer/reference after the call returns.

The contract MUST distinguish at minimum:

```text
borrowed for call duration
retained after call
```

If foreign code retains a mutable alias to invariant-bearing native storage, simple post-call validation can be insufficient.

Safe interop can require:

- copying;
- opaque wrappers;
- no-retention guarantee;
- repeated validation;
- unsafe contract.

The exact rules belong to RFC-008.

#### 6.9.7 Foreign callbacks

When an Anuy function is passed to Go and later called from Go:

```text
Go
 ↓
Anuy callback
```

the callback boundary is a new foreign entry point.

Arguments do not become native merely because the callback is written in Anuy.

The generated wrapper MUST:

- project/validate inputs;
- preserve Anuy invariants;
- map results back into the Go ABI.

#### 6.9.8 Unsafe callbacks

If the callback inputs cannot be safely validated, or the caller is obliged to uphold external invariants, the callback contract MUST be unsafe/trusted.

A safe Anuy callback body MUST NOT silently receive stronger types than the external caller actually guarantees.

#### 6.9.9 Exporting Anuy to Go

The reverse direction:

```text
Anuy
 ↓
Go
```

is also a trust boundary.

A Go consumer can:

- retain pointers;
- manufacture representations;
- call methods in unexpected orders;
- return modified values.

The generated public Go ABI MUST account for the possibility of these values re-entering Anuy.

RFC-008/RFC-009 define the concrete wrappers.

#### 6.9.10 Mixed `.go` + `.anuy` packages

The presence of `.go` and `.anuy` files in one Go package does not turn the Go code into safe Anuy code.

With respect to the stronger Anuy invariants, `.go` source remains foreign.

This matters because Go code can:

- construct zero values;
- assign invalid enum discriminants;
- create typed nil interfaces;
- ignore intended Anuy initialization rules.

A mixed package does not bypass the trust boundary.

### 6.10 Trusted Contracts

#### 6.10.1 Foreign result semantics

RFC-005 distinguishes strict native fallible results from foreign partial-result APIs.

The foreign contract MUST tell the compiler whether to treat:

```text
err != nil
→
success values unavailable
```

or the foreign API admits meaningful partial values.

This is a contract property, not a compiler guess.

#### 6.10.2 Trusted contract declarations

Sometimes a package author creates Anuy metadata asserting a stronger guarantee about a foreign API.

For example:

```text
Go returns *T
documentation promises non-null
```

If the declaration turns this into the native:

```text
*T
```

without validation, the declaration contains an unsafe assumption.

Such a site MUST be explicitly marked/auditable.

RFC-008 will define the syntax.

#### 6.10.3 Contracts should be local

Trusted foreign assertions SHOULD live as close as possible to the boundary definition rather than be repeated at every call site.

Preferred:

```text
one audited interop contract
↓
many safe calls
```

rather than:

```text
unsafe at every use site
```

This localizes the proof obligation.

#### 6.10.4 Unsafe abstraction boundary

A good unsafe abstraction has the shape:

```text
small unsafe core
↓
validated invariant
↓
large safe API
```

Anuy tooling SHOULD help maintain this design.

### 6.11 Tooling and Policy

#### 6.11.1 Tooling: unsafe searchability

All source-level unsafe sites MUST be syntactically searchable.

At minimum:

```text
unsafe {
unsafe func
```

Tooling SHOULD be able to show:

- all unsafe blocks;
- all unsafe functions;
- trusted foreign contracts;
- raw-representation operations;
- call sites of unsafe functions.

#### 6.11.2 No implicit unsafe

The compiler MUST NEVER silently insert an unsafe assertion to make a program type-check.

If safe projection is impossible, compilation MUST fail or the API MUST remain in the weaker form.

The following logic is not allowed:

```text
probably non-null
probably valid enum
probably initialized
```

#### 6.11.3 Project policy

Language semantics admits unsafe.

Tooling MAY provide a policy:

```text
deny unsafe
warn unsafe
allow unsafe
```

for example for packages, projects, or CI.

This is a tooling policy, not a validity distinction of the core language.

#### 6.11.4 Standard library policy

The Anuy standard/support libraries MAY use unsafe to implement low-level primitives.

But a public safe function MUST hide the proof obligation from the caller.

An unsafe implementation does not automatically turn an API into an unsafe API.

#### 6.11.5 Compiler assumptions

After a safe proof, a validation, or an unsafe assertion, the compiler is entitled to optimize based on the invariant.

For example, after:

```anuy
unsafe {
    var user = assume_non_nil(value)
}
```

the compiler is not required to keep further defensive nil checks merely because the original value was nullable.

This makes false unsafe promises genuinely dangerous.

#### 6.11.6 Unsafe is not merely “may panic”

Some ordinary safe operations can panic under Go semantics.

That by itself does not make them unsafe.

`unsafe` means something else:

> correctness depends on an invariant outside the guarantees currently proven by the language.

Consequently:

```text
possible panic
≠
unsafe
```

#### 6.11.7 Panic is not validation

The programmer MUST NOT treat:

```text
if invalid, generated code probably panics
```

as equivalent to a valid safe boundary.

A safe boundary MUST preserve the source invariants before a malformed representation becomes a native value.

A synthetic trap can be used as an implementation of the check, but not as an excuse for unchecked projection.

### 6.12 Concepts and the Safety Theorem

#### 6.12.1 Foreign invalid state

If the boundary detects a value that violates an Anuy invariant, it is called:

```text
foreign invalid state
```

Such a state is not an additional inhabitant of the native type.

For example, an invalid enum discriminant does not extend:

```text
Values(Color)
```

It remains a rejected foreign representation.

#### 6.12.2 Foreign versus nullable

`foreign` and `nullable` are different concepts.

```text
T?
```

says:

> semantic nil is an allowed value.

Foreign says:

> external origin may not yet justify native invariants.

Foreign uncertainty MUST NOT be modeled through `nil` when the actual uncertainty is not nullability.

#### 6.12.3 Foreign versus uninitialized

Likewise:

```text
foreign ≠ uninitialized
```

Uninitialized is a compile-time state of a binding.

A foreign value exists at runtime but requires boundary interpretation.

These concepts MUST NOT be mixed.

#### 6.12.4 Foreign versus unsafe

Foreign code is not necessarily unsafe.

Unsafe code is not necessarily foreign.

An example of safe foreign:

```text
Go Add(int,int) int
```

An example of an unsafe native assertion:

```anuy
unsafe {
    var x = assume_non_nil(value)
}
```

This is an important conceptual separation.

#### 6.12.5 Safe boundary theorem

RFC-007 requires the following language property:

> If execution enters safe Anuy with valid native inputs, and all unsafe contracts and trusted foreign contracts used by the program are true, safe Anuy cannot construct or observe a value violating its declared source-type invariants.

This is precisely what the stronger guarantees of Anuy mean.

#### 6.12.6 Consequence of unsafe violation

If an unsafe/trusted contract is false, the theorem no longer applies.

This is the only point where the programmer consciously takes proof responsibility upon themselves.

Therefore unsafe sites must be:

- explicit;
- small;
- searchable;
- documentable.

## 7. Interaction with Other RFCs

### 7.1 RFC-001

The RFC-001 invariant:

```text
uninitialized ≠ zero value
```

holds even in unsafe code.

Unsafe does not allow reading a compiler-uninitialized binding.

A foreign zeroed aggregate can be accepted as native only if it actually satisfies the semantic invariants or the boundary validates it.

### 7.2 RFC-002

The RFC-002 invariant:

```text
T is non-null
T? may contain nil
```

holds at the foreign boundary.

A foreign possibly-null pointer MUST:

- project as `T?`;
- validate;
- or use an explicit trusted assertion.

`assume_non_nil` is an unsafe escape hatch, not part of the normal nullability flow.

### 7.3 RFC-003

Unsafe does not change declaration/assignment semantics.

Inside unsafe:

```text
var
    still declares

=
    still assigns
```

Shadowing/scope rules remain the same.

### 7.4 RFC-004

Unsafe does not override explicit interface conformance.

Go type → Anuy interface requires the designated foreign/adaptor mechanism.

Unsafe cannot arbitrarily create a method implementation.

### 7.5 RFC-005

The foreign contract describes the error protocol:

```text
strict
partial/raw
```

Unsafe does not allow silently ignoring an error-result.

Error checking remains enforced.

### 7.6 RFC-006

The native enum guarantee:

```text
value ∈ declared variants
```

MUST hold at the foreign boundary.

An invalid Go discriminant does not automatically become a native enum.

Unsafe construction of an enum means the explicit promise that the discriminant is valid.

### 7.7 RFC-008

RFC-008 MUST make this trust model concrete for Go interoperability:

- Go → Anuy type projection;
- nullability inference;
- typed nil;
- enum wrappers;
- callbacks;
- mixed packages;
- retained aliases;
- partial error results;
- contract declaration syntax;
- safe vs unsafe imported operations.

RFC-007 defines the policy; RFC-008 — the concrete Go mapping.

### 7.8 RFC-009

RFC-009 MUST define the privileged representation operations:

- raw layouts;
- pointer conversion;
- unchecked representation casts;
- generated validation;
- ABI wrappers;
- invalid-state traps;
- SourceMap treatment.

Representation details cannot weaken the RFC-007 safety model.

## 8. Diagnostics and Tooling

### 8.1 SourceMap

Unsafe source constructs have ordinary source locations.

Generated code that performs foreign validation SHOULD have:

```text
SourceKind = Validation
```

The generated bridge:

```text
SourceKind = InteropWrapper
```

Synthetic raw-conversion helpers MUST map to the original unsafe site.

### 8.2 Diagnostics Catalog

The identifiers D-1…D-5 are local references within this RFC; the message forms are illustrative; stable `ANUY####` codes follow RFC-011 policy.

#### 8.2.1 D-1 — Unsafe Operation Outside Block

Recommended:

```text
error: assume_non_nil requires an unsafe context

    var user = assume_non_nil(value)
               ^^^^^^^^^^^^^^^^^^^^^^

the compiler cannot prove that value is non-null

use normal control-flow validation, or explicitly accept
the invariant responsibility inside:

    unsafe {
        ...
    }
```

#### 8.2.2 D-2 — Unsafe Call

```text
error: call to unsafe function requires an unsafe context

    var buffer = fromRawAddress(ptr)
                 ^^^^^^^^^^^^^^^^^^^

the caller must uphold the function's safety contract
```

#### 8.2.3 D-3 — Foreign Contract Mismatch

Conceptual:

```text
error: foreign value cannot be projected as Color

the foreign representation may contain discriminants
outside the declared variants of Color

validate the value or provide an explicit trusted contract
```

#### 8.2.4 D-4 — Possibly-null Foreign Value

```text
error: foreign result may be nil

Go signature:
    *User

safe Anuy projection:
    *User?

a non-null *User requires validation or an explicit
trusted non-null contract
```

#### 8.2.5 D-5 — Unsafe Does Not Bypass Initialization

```text
error: value is uninitialized

unsafe does not permit reading an uninitialized binding

initialize the value before use
```

This diagnostic SHOULD be intentional so that the programmer understands the boundaries of unsafe.

#### 8.2.6 R — Redundant Unsafe Assertion

Advisory (Warning; develops §6.6.8): an assertion over an already-proven invariant is ergonomic noise that hides real unsafe sites. Proof is preferable to unsafe.

```text
warning: assume_non_nil on a value already known to be non-null

    var user = assume_non_nil(validated)
               ^^^^^^^^^^^^^^^^^^^^^^^^

the compiler has already proven that validated is non-null;
drop the assertion and the unsafe context
```

## 10. Rejected Alternatives

### 10.1 All Go calls are unsafe

Rejected:

```text
every call into Go requires unsafe
```

Reasons:

- almost the entire Anuy ecosystem is built on Go;
- most Go operations do not violate the stronger invariants;
- unsafe noise hides the assumptions that really matter;
- interoperability becomes impractical.

Safety is determined by the contract, not by the implementation language.

### 10.2 Foreign taint everywhere

Rejected pervasive:

```text
foreign T
```

propagation through every expression.

Reasons:

- infects ordinary code;
- significantly complicates generics and APIs;
- most foreign primitive data does not need taint;
- boundary validation is a more local model.

### 10.3 Unsafe disables checker

Rejected semantics:

```text
unsafe {
    anything goes
}
```

Such semantics would destroy the compiler's ability to build semantic IR and optimize the program.

Unsafe MUST be a capability for specific proof escapes, not an escape from the language.

### 10.4 Postfix non-null assertion

Not introduced:

```text
value!
```

or:

```text
value!!
```

as a normal expression.

Such an operation hides the unsafe proof obligation too easily.

The explicit form is used:

```anuy
unsafe {
    assume_non_nil(value)
}
```

### 10.5 Implicit trusted contracts

The compiler does not trust prose documentation automatically.

If the external type system does not guarantee the invariant, the stronger projection requires:

- validation;
- or an explicit trust declaration.

Documentation by itself is not a machine proof.

### 10.6 Validation after native observation

It is not allowed to:

```text
construct native value
↓
let safe code observe it
↓
validate later
```

Validation MUST happen **before** the semantic upgrade to the native stronger type.

Otherwise the safety invariant is already violated.

### 10.7 Validate once despite mutable foreign alias

A single validation is not a proof forever if uncontrolled foreign code can later change the representation.

The alias/mutation contract is part of the trust model.

## 12. Open Questions

- **OQ-1 Contract declaration syntax.** The syntax of trusted foreign contracts and unsafe documentation (Sections 6.3.4, 6.7.5, 6.10.2) is defined by RFC-008.
- **OQ-2 Representation intrinsics.** The exact raw-representation/unchecked-cast intrinsic syntax (Section 6.8.7) is defined by RFC-009.

## 13. Normative Summary

RFC-007 v1 fixes:

1. `native` and `foreign` are semantic trust concepts.
2. Foreign origin by itself does not mean an unsafe operation.
3. Go calls MAY be fully safe.
4. A safe value of type `T` means that all invariants of `T` hold.
5. A foreign value can be projected to `T` only with a sufficient guarantee.
6. The guarantee can come from static proof, a representation guarantee, runtime validation, or an explicit trusted assertion.
7. Runtime validation is a safe operation.
8. An unsafe assertion moves proof responsibility to the programmer.
9. `unsafe { ... }` is a lexical privileged context.
10. Unsafe does not create a runtime mode.
11. Unsafe does not disable ordinary type checking.
12. Unsafe does not disable definite initialization.
13. Unsafe does not disable error handling.
14. Unsafe does not disable exhaustive matching.
15. Unsafe does not create interface conformance.
16. An uninitialized binding must not be read even inside an unsafe block.
17. `assume_non_nil(T?) -> T` is an unsafe assertion.
18. `assume_non_nil` is not required to perform a runtime check.
19. A false unsafe assumption violates the language safety contract.
20. Values established through unsafe MAY escape unsafe block.
21. Safe API MAY internally contain unsafe implementation.
22. `unsafe func` denotes caller-side unchecked preconditions.
23. Calling `unsafe func` requires unsafe context.
24. The body of an `unsafe func` is not an implicit unsafe block.
25. Exported unsafe functions MUST document caller safety obligations.
26. Representation-level unchecked operations require an unsafe context.
27. An unsafe conversion must still be backend/representation-legitimate.
28. Safe Anuy never silently upgrades uncertain foreign state.
29. Foreign contracts describe mapping foreign ABI → Anuy semantics.
30. Foreign contracts distinguish structural, checked and trusted guarantees.
31. A trusted guarantee without validation is an explicit unsafe responsibility.
32. Foreign mutation can invalidate previously established invariants.
33. Safe code can reuse the invariant after foreign mutation only when the contract guarantees preservation or validation is repeated.
34. Retained mutable foreign aliases require explicit contract handling.
35. Go callbacks into Anuy are foreign entry boundaries.
36. `.go` code in mixed packages remains foreign with respect to stronger Anuy invariants.
37. Nullable uncertainty, foreign uncertainty and uninitialized state are distinct concepts.
38. Invalid foreign representations are not additional inhabitants of native types.
39. Invalid enum discriminants must be rejected or explicitly unsafe-trusted.
40. Possibly-null foreign pointer must not silently become non-null Anuy pointer.
41. Foreign typed-nil interface states require contract handling.
42. Foreign strict/partial error semantics are explicit contract properties.
43. Compiler never inserts hidden unsafe assumptions merely to make code type-check.
44. Tooling SHOULD make unsafe and trusted sites searchable.
45. Project tooling MAY deny or warn on unsafe usage.
46. Generated boundary validation uses canonical SourceMap infrastructure.
47. RFC-008 defines concrete Go interop contracts.
48. RFC-009 defines concrete representation-level unsafe operations.

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (safety boundary, unsafe philosophy);
- RFC-001 — Values, Initialization and Trust (uninitialized is inaccessible to unsafe);
- RFC-002 — Nullability and Nil Safety (foreign ≠ nullable, `assume_non_nil`);
- RFC-003 — Variables, Assignment and Scope (scope rules in unsafe);
- RFC-004 — Interfaces and Explicit `impl` (unsafe does not create conformance);
- RFC-005 — Error Handling and Propagation (unsafe does not disable error checking);
- RFC-006 — Enums and Exhaustive Matching (foreign discriminants);
- RFC-008 — Go Interoperability (contract syntax, Go mapping);
- RFC-009 — Lowering and Generated Go Contract (representation intrinsics);
- RFC-011 — Diagnostics, Debugging and Tooling (SourceMap, codes, searchability).
