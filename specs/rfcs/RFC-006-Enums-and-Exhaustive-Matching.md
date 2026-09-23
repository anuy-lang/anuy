# RFC-006 — Enums and Exhaustive Matching

**Status:** Accepted
**RFC:** 006
**Title:** Enums and Exhaustive Matching
**Language:** Anuy
**Area:** Type System / Enums / Pattern Matching / Control Flow
**Version:** 6
**Date:** 2026-09-24
**Requires:** RFC-000, RFC-001, RFC-002, RFC-003, RFC-004, RFC-005, RFC-007, RFC-008, RFC-009, RFC-012
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-006 was reviewed against RFC-000 (closed high-value constructs instead of general mechanisms), RFC-001 (no implicit default/zero variant), RFC-002 (nullable enums) and RFC-004 (enum methods/interfaces). Delegations: foreign validation policy — RFC-007/008/009; representation and ABI — RFC-009; payload/sum types — RFC-012 (Section 7.9).

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

Version 3, 2026-09-20: the declaration syntax changed to `type N enum { … }` — a single declarative framework `type N <kind>` (like `type N struct`), following the Go-first principle of framework reuse (RFC-014 v3); variant references `Enum.Variant` and representation (RFC-009 §6.4) unchanged (owner decision, 2026-09-20).

Version 4, 2026-09-20: the surface form of §6.3–6.4 changed from `match … => …` to Go `switch/case` — a Dart 3 / Java 14 precedent of upgrading an existing construct; lowering generates a Go switch anyway (RFC-009 §6.7); exhaustiveness semantics (§6.3.4–6.3.7, §6.4.7) unchanged; `default` over a native enum is forbidden. Examples outside §6.3–6.4 (§6.5–6.12) keep the v2/v3 spelling until the corresponding sections are revised; the canonical form is defined by §6.3.1/§6.4.1 (owner decision, 2026-09-20).

Version 5, 2026-09-20: §6.5 moved to the switch/case surface (v4); clarified: variant-arm refinement facts (§6.5.4) participate in ordinary control-flow analysis and are implemented in the experimental layer — a variant arm proves `scrutinee != nil` for D-5; a `nil` arm on a non-null enum is a compile error (§6.5.3) (owner decision, 2026-09-20).

2026-09-24: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 5 → 6.

---

## 1. Abstract

Anuy introduces closed nominal enums without payload variants and exhaustive `match`: every native enum value is exactly one declared variant, and `match` MUST explicitly cover every variant, which makes adding a variant a deliberate source-breaking change. An enum supports equality, methods and explicit interface conformance, is not a number, and has no implicit default. Nullable enums (`E?`) match exhaustively with an explicit `nil` arm; lowering uses ordinary Go dispatch with a synthetic invalid branch to protect the foreign boundary.

## 2. Solutions

> **Every native enum value is exactly one declared variant.**

> **A switch over a native enum must explicitly cover every variant.**

> **Every native enum variant must be named explicitly in a `switch`.**

> **Invalid foreign discriminants must never silently become valid native enum values.**

> **An enum is a closed set of explicitly named values, and `switch` is a checked statement that the programmer has considered every one of them.**

> **Closed values. Exhaustive decisions. Breaking changes where intent must be reconsidered.**

## 3. Motivation

Go models a closed set of values through `type + const` — and cannot guarantee anything: an arbitrary value of the type is always possible, `switch` does not require completeness, and adding a constant silently slips past every existing `switch`. The value of an enum is not in named constants but in a checkable contract: "the set is closed, and every decision covers it." Anuy makes this contract part of the language: the compiler proves exhaustiveness and turns enum evolution into explicit decision points.

## 4. Goals

RFC-006 MUST provide:

1. closed finite sets of named values;
2. nominal enum identity;
3. no way to accidentally create an undeclared native variant;
4. exhaustive compile-time matching;
5. source-breaking detection when a variant is added;
6. integration with nullability;
7. integration with definite initialization;
8. straightforward Go lowering;
9. predictable foreign-boundary behavior;
10. no separate runtime mechanism.

## 5. Non-goals

RFC-006 v1 does not introduce:

- payload variants;
- general sum types;
- Rust-style algebraic data types;
- pattern destructuring;
- match guards;
- wildcard enum arms;
- user-defined numeric discriminants;
- implicit enum ↔ integer conversion;
- implicit enum ↔ string conversion;
- open/extensible enums;
- inheritance between enums;
- dynamic variant registration.

## 6. Specification

### 6.1 Declaration and Identity

#### 6.1.1 Enum declaration

Syntax:

```anuy
type Color enum {
    Red
    Green
    Blue
}
```

Each declared name is a variant of the type:

```text
Color
```

That is:

```anuy
var color Color = Color.Red
```

is valid.

A variant by itself is not a separate type.

```text
Color.Red
```

— a value of type `Color`.

#### 6.1.2 Nominal identity

Enums are nominal types.

For example:

```anuy
type Color enum {
    Red
    Green
}

type Signal enum {
    Red
    Green
}
```

despite identical variant names:

```text
Color ≠ Signal
```

and:

```anuy
var color Color = Signal.Red
```

is a compile error.

#### 6.1.3 Qualified variant names

The canonical reference uses the enum name:

```anuy
Color.Red
Color.Green
Color.Blue
```

Bare:

```anuy
Red
```

is not an ordinary variant reference in v1.

This prevents namespace ambiguity and makes the variant's type obvious when reading code.

#### 6.1.4 Variant names

Variant names are unique within an enum.

Invalid:

```anuy
type State enum {
    Ready
    Ready
}
```

The variant namespace belongs to the enum, so the following is allowed:

```anuy
type ConnectionState enum {
    Ready
}

type JobState enum {
    Ready
}
```

and references remain unambiguous:

```anuy
ConnectionState.Ready
JobState.Ready
```

#### 6.1.5 Non-empty enums

A native enum in v1 MUST have at least one variant.

Invalid:

```anuy
type Impossible enum {
}
```

Uninhabited types and `never` semantics are outside the scope of RFC-006.

#### 6.1.6 No payload variants

In v1 a variant is only a named member of a closed set.

Valid:

```anuy
type TokenKind enum {
    Identifier
    Number
    Plus
    Minus
}
```

Invalid:

```anuy
type Token enum {
    Identifier(string)
    Number(int)
}
```

If variant-specific data is needed today, the programmer uses an ordinary aggregate:

```anuy
type TokenKind enum {
    Identifier
    Number
}

struct Token {
    kind TokenKind
    text string
}
```

General payload variants MAY be added by the future RFC-012.

### 6.2 Values and Operations

#### 6.2.1 Initialization

Enums follow RFC-001.

```anuy
var color Color
```

means:

> the binding exists, but the enum value has not been created yet.

It does not mean:

```text
Color.Red
```

and it does not mean any compiler-selected default variant.

Reading:

```anuy
print(color)
```

before definite initialization is a compile error.

#### 6.2.2 No default variant

Anuy has no implicit enum default.

The first variant:

```anuy
Red
```

is not a special default value.

Declaration order does not determine source-level initialization semantics.

If the programmer wants a default:

```anuy
var color = Color.Red
```

they write it explicitly.

#### 6.2.3 Enum assignment

Ordinary assignment:

```anuy
var color = Color.Red

color = Color.Blue
```

is valid.

The assigned value MUST be assignable to the same enum type.

There is no implicit conversion between distinct enums.

#### 6.2.4 Equality

Enum values always support:

```anuy
==
!=
```

For example:

```anuy
if color == Color.Red {
    ...
}
```

Enum comparison compares semantic variants.

#### 6.2.5 No ordering

Native enums in v1 do not automatically get:

```text
<
<=
>
>=
```

Declaration order is not a source-language ordering relation.

If an application needs ordering, it must be defined explicitly through methods/functions.

#### 6.2.6 No arithmetic

Enums are not integers.

Invalid:

```anuy
color + 1
color - 1
color++
```

The presence of an integer-like representation in generated Go does not change source semantics.

#### 6.2.7 No integer conversion

Safe Anuy v1 does not provide:

```anuy
var n int = int(color)
var color Color = Color(n)
```

as an ordinary conversion mechanism.

The numeric representation is a backend detail.

Raw representation access belongs to the `unsafe`/interop RFC.

#### 6.2.8 No string conversion

A variant name is not automatically a string value.

Invalid assumption:

```anuy
var text string = color
```

If an application needs a textual representation, the programmer can define a method:

```anuy
func (color Color) String() string {
    return match color {
        Color.Red => "red"
        Color.Green => "green"
        Color.Blue => "blue"
    }
}
```

and, if needed, explicit interface conformance.

#### 6.2.9 Methods on enums

An enum is an ordinary named type and can have methods.

```anuy
func (color Color) IsWarm() bool {
    return color == Color.Red
}
```

Pointer receivers also follow the general RFC-004 rules, although value receivers are usually more natural for enum values.

#### 6.2.10 Interfaces

An enum can explicitly implement interfaces:

```anuy
func (color Color) String() string {
    ...
}

impl fmt.Stringer for Color
```

The presence of methods by itself does not create native interface conformance.

RFC-004 applies without special exceptions.

### 6.3 `switch` Basics

#### 6.3.1 `switch`

The primary enum control-flow construct:

```anuy
switch color {
    case Color.Red:
        ...

    case Color.Green:
        ...

    case Color.Blue:
        ...
}
```

The scrutinee:

```text
color
```

MUST have a statically known enum type or a nullable enum type.

#### 6.3.2 Exactly-once evaluation

The switch scrutinee is evaluated exactly once.

```anuy
switch nextColor() {
    ...
}
```

cannot call:

```text
nextColor()
```

multiple times.

The compiler MAY create a synthetic temporary.

Observable semantics MUST be exactly-once.

#### 6.3.3 Exactly one arm executes

For a valid native enum value, exactly one arm is always selected.

Variants are mutually exclusive.

There is no fallthrough.

#### 6.3.4 Exhaustiveness

For:

```anuy
type Color enum {
    Red
    Green
    Blue
}
```

the switch:

```anuy
switch color {
    case Color.Red:
        ...

    case Color.Green:
        ...
}
```

is a compile error.

The diagnostic MUST report (Section 8.1, D-1):

```text
missing variant:
    Color.Blue
```

#### 6.3.5 Duplicate arms

A variant cannot be handled twice.

Invalid:

```anuy
switch color {
    case Color.Red:
        ...

    case Color.Red:
        ...

    case Color.Green:
        ...

    case Color.Blue:
        ...
}
```

The compiler reports a duplicate/unreachable arm (Section 8.1, D-3).

#### 6.3.6 No wildcard arm for native enums

RFC-006 v1 intentionally does not introduce:

```anuy
default:
    ...
```

for the native enum switch.

The reason is the API evolution guarantee.

If allowed:

```anuy
switch color {
    case Color.Red:
        ...
    case _:
        ...
}
```

then adding a new enum variant would silently fall into the wildcard.

This contradicts the main goal of exhaustive matching.

Therefore:

> **Every native enum variant must be named explicitly in a `switch`.**

#### 6.3.7 Why wildcard is intentionally absent

Suppose:

```anuy
type Permission enum {
    Read
    Write
}
```

and the application writes:

```anuy
switch permission {
    case Permission.Read:
        allowRead()
    case Permission.Write:
        allowWrite()
}
```

Later the library adds:

```anuy
Admin
```

The compiler MUST force the application to decide:

```text
What does Admin mean here?
```

This is a feature, not a compatibility defect.

#### 6.3.8 If programmer intentionally wants “everything else”

For a non-exhaustive policy the programmer can use ordinary conditional logic:

```anuy
if color == Color.Red {
    ...
} else {
    ...
}
```

Such code intentionally does not receive the enum-evolution guarantee.

The distinction between:

```text
match
```

and:

```text
if
```

is semantically useful:

```text
switch
    = I have considered every variant

if
    = I am testing a condition
```

### 6.4 Switch Forms and Typing

#### 6.4.1 Switch arm syntax

Statement form:

```anuy
switch state {
    case State.Starting:
        start()

    case State.Running:
        run()

    case State.Stopped:
        stop()
}
```

Each arm has its own lexical scope.

#### 6.4.2 Value-producing switch

A `switch` can also produce a value.

```anuy
var text = switch color {
    case Color.Red:
        "red"
    case Color.Green:
        "green"
    case Color.Blue:
        "blue"
}
```

All reachable arms MUST produce a value compatible with a single switch result type.

#### 6.4.3 Switch result typing

For:

```anuy
var value = switch enumValue {
    ...
}
```

the compiler determines a single result type by the ordinary expression typing rules.

For example:

```anuy
var code = switch color {
    case Color.Red:
        1
    case Color.Green:
        2
    case Color.Blue:
        3
}
```

has type:

```text
int
```

#### 6.4.4 Contextual switch typing

If the switch is in a context with an expected type:

```anuy
var value string? = switch state {
    case State.Ready:
        "ready"
    case State.Missing:
        nil
}
```

each arm MUST be assignable to the expected result type.

The usual widening:

```text
T → T?
```

applies per RFC-002.

#### 6.4.5 No implicit arm fallthrough

Arms are independent.

There is no syntax/semantics analogous to Go:

```go
fallthrough
```

for `switch`.

If several variants must perform the same operation, v1 requires explicit arms.

For example:

```anuy
switch state {
    case State.Starting:
        handleActive()
    case State.Running:
        handleActive()
    case State.Stopped:
        handleStopped()
}
```

A future RFC MAY consider multi-pattern arms, but v1 does not require them.

#### 6.4.6 Arm order

Arm order does not affect matching semantics, because variants are mutually exclusive.

Tooling MAY recommend declaration order for readability.

Compiler correctness does not depend on arm order.

#### 6.4.7 No guards in v1

Not supported:

```anuy
State.Ready if condition => ...
```

Guards complicate exhaustiveness:

a variant may be syntactically present, but the guard may be false.

Therefore the enum switch in v1 uses only unconditional variant arms.

The programmer writes nested control flow:

```anuy
switch state {
    case State.Ready:
        if condition {
            ...
        } else {
            ...
        }

    ...
}
```

### 6.5 Nullable Enums

#### 6.5.1 Nullable enums

RFC-002 allows:

```text
Color?
```

Therefore `switch` knows how to exhaustively handle a nullable enum.

Example:

```anuy
var color Color? = ...

switch color {
    case nil:
        noColor()

    case Color.Red:
        ...

    case Color.Green:
        ...

    case Color.Blue:
        ...
}
```

#### 6.5.2 Exhaustiveness for nullable enum

For:

```text
E?
```

the exhaustive set is:

```text
{ nil } ∪ Variants(E)
```

Therefore the absence of a `nil` arm:

```anuy
switch color {
    case Color.Red:
        ...
    case Color.Green:
        ...
    case Color.Blue:
        ...
}
```

is a compile error if:

```text
color : Color?
```

#### 6.5.3 `nil` arm on non-null enum

If:

```text
color : Color
```

then:

```anuy
case nil:
    ...
```

is a compile error / an unreachable arm.

A non-null enum value cannot be `nil`.

#### 6.5.4 Nullability refinement inside arms

For:

```text
color : Color?
```

in the arm:

```anuy
case Color.Red:
    ...
}
```

the compiler knows:

```text
color != nil
color == Color.Red
```

In:

```anuy
case nil:
    ...
}
```

the compiler knows:

```text
color == nil
```

These facts participate in ordinary control-flow analysis.

#### 6.5.5 Variant refinement

In the arm:

```anuy
case Color.Red:
    ...
}
```

the compiler records the semantic fact:

```text
color == Color.Red
```

Even without payload variants this is useful for:

- diagnostics;
- unreachable code detection;
- control-flow reasoning;
- future extension of the pattern system.

### 6.6 Match Integration: Initialization and `try`

#### 6.6.1 Match and definite initialization

Exhaustiveness integrates with RFC-001.

For example:

```anuy
var text string

match color {
    Color.Red => {
        text = "red"
    }

    Color.Green => {
        text = "green"
    }

    Color.Blue => {
        text = "blue"
    }
}

print(text)
```

is valid.

Why:

```text
every reachable match arm initializes text
+
match is exhaustive
=
text definitely initialized after match
```

#### 6.6.2 Missing initialization in one arm

Invalid:

```anuy
var text string

match color {
    Color.Red => {
        text = "red"
    }

    Color.Green => {
        text = "green"
    }

    Color.Blue => {
    }
}

print(text)
```

The compiler reports that `text` is not initialized after `Color.Blue`.

#### 6.6.3 Diverging arms

If an arm does not reach the join point:

```anuy
match state {
    State.Ready => {
        value = compute()
    }

    State.Invalid => {
        return error invalidState()
    }
}

use(value)
```

then the returning arm does not require initialization of `value`.

Ordinary reachability/dataflow rules apply to the match control-flow graph.

#### 6.6.4 Match and `try`

`try` can be used inside an arm per RFC-005:

```anuy
match source {
    Source.Local => {
        var data = try loadLocal()
        use(data)
    }

    Source.Remote => {
        var data = try loadRemote()
        use(data)
    }
}
```

`try` propagates from the enclosing function, not from the match.

#### 6.6.5 Match expression and `try`

Since RFC-005 forbids hidden nested `try`, the form:

```anuy
var value = match source {
    Source.Local => try loadLocal()
    Source.Remote => try loadRemote()
}
```

is not allowed in v1.

Statement control flow or a helper function is used instead.

This preserves the visibility of early-return semantics.

### 6.7 Scopes, Aliases and Generics

#### 6.7.1 Match arm scope

Each block arm creates a lexical scope:

```anuy
match state {
    State.Ready => {
        var value = 1
    }

    State.Stopped => {
        var value = 2
    }
}
```

The two `value` bindings are distinct.

They do not exist after the match.

RFC-003 shadowing rules apply as usual.

#### 6.7.2 Matching aliases

A type alias preserves enum identity.

Conceptually:

```anuy
type StatusAlias = Status
```

can use the variants:

```anuy
Status.Ready
```

and match exhaustiveness is determined by the original enum declaration.

An alias does not create a second closed variant set.

#### 6.7.3 Defined types do not inherit enum-ness implicitly

Only:

```anuy
type Name enum {
    ...
}
```

creates a new native enum.

Creating a distinct named type based on the enum representation MUST NOT automatically copy the variant set.

This prevents the emergence of two nominal enum types with an unclear relation.

Exact general defined-type rules are defined by the type declaration RFC.

#### 6.7.4 Generic code

An enum can be used as an ordinary generic argument:

```anuy
Container[Color]
```

But v1 does not introduce generic enum declarations:

```anuy
enum State[T] {
    ...
}
```

since, in the absence of payload variants, a type parameter has no useful role.

#### 6.7.5 Match on generic type parameters

In v1 the scrutinee of a `match` MUST resolve to a concrete native enum declaration after aliases.

For example, an arbitrary:

```text
T
```

cannot be exhaustively matched merely because a generic constraint makes some assumptions.

This keeps the closed-world exhaustiveness analysis simple.

A future generic-pattern RFC MAY extend the rule.

### 6.8 Visibility and Evolution

#### 6.8.1 Variant visibility

A variant has no visibility independent of the enum.

The variant set is part of the enum's definition.

If an enum is accessible to a consumer package, the compiler MUST know its entire variant set.

This is necessary for exhaustive matching.

Consequently a public enum cannot have a "secret variant" that external Anuy code is unable to name.

#### 6.8.2 Export metadata

Compiler package metadata MUST export, for an accessible enum:

```text
EnumID
EnumName
VariantIDs
VariantNames
DeclarationOrder
Source locations where available
```

The downstream Anuy compiler uses this data for exhaustiveness.

Generated Go source is not the canonical source of enum semantics.

#### 6.8.3 Adding a variant is source-breaking

Suppose a package exposes:

```anuy
type State enum {
    Ready
    Stopped
}
```

A consumer:

```anuy
match state {
    State.Ready => ...
    State.Stopped => ...
}
```

The library changes to:

```anuy
type State enum {
    Ready
    Paused
    Stopped
}
```

Consumer compilation fails:

```text
non-exhaustive match

missing:
    State.Paused
```

This is an intentional compatibility contract.

#### 6.8.4 Removing a variant

Removing a variant is also source-breaking.

Existing references:

```anuy
State.Paused
```

stop resolving.

Existing match arms become invalid.

#### 6.8.5 Renaming a variant

Renaming a variant is a source-breaking change.

Variant identity is not derived from declaration position.

The source-level name is part of the enum API.

#### 6.8.6 Reordering variants

At the source semantic level match behavior does not depend on order.

However, the generated representation MAY use declaration order.

Therefore the ABI implications of reordering are defined by RFC-009.

Until lowering is finally fixed, tooling SHOULD treat reordering as a potentially ABI-sensitive operation for exported enums.

#### 6.8.7 Public API evolution

For native Anuy consumers:

```text
adding variant
```

is intentionally source-breaking.

For generated Go consumers exact compatibility depends on generated representation and is handled by RFC-009.

This difference is acceptable:

> Anuy provides stronger compile-time guarantees than raw Go consumers receive.

### 6.9 Representation

#### 6.9.1 Representation principle

Enum source semantics does not depend on the integer representation.

Conceptually the compiler can lower an enum to:

```go
type Color uint32
```

with generated constants.

But the source language only knows:

```text
Color.Red
Color.Green
Color.Blue
```

It does not know:

```text
1
2
3
```

#### 6.9.2 Invalid representation state

Go storage inevitably has a zero representation.

An Anuy enum is not required to have a semantic variant corresponding to that zero.

This continues RFC-001:

> storage representation is not the same thing as a valid Anuy value.

The compiler MAY reserve the backend zero representation as an invalid/uninitialized/foreign-invalid state.

Safe Anuy code cannot create or observe it as an initialized enum value.

#### 6.9.3 No implicit `Unknown`

The compiler does not add a source-visible:

```text
Unknown
Invalid
Zero
```

variant.

If the domain requires:

```anuy
Unknown
```

the programmer declares it explicitly:

```anuy
type State enum {
    Unknown
    Ready
    Stopped
}
```

This is application semantics, not a language storage mechanism.

#### 6.9.4 Suggested canonical Go representation

RFC-006 recommends, but leaves the final ABI commitment to RFC-009:

```go
type Color uint32

const (
    ColorRed   Color = 1
    ColorGreen Color = 2
    ColorBlue  Color = 3
)
```

The representation value:

```text
0
```

does not correspond to a native variant.

Exact generated identifier spelling belongs to RFC-009.

#### 6.9.5 Why reserve zero

Reserving zero gives a useful invariant:

```text
zeroed Go storage
≠
valid initialized Anuy enum value
```

This matches the general language philosophy:

```text
uninitialized ≠ valid zero/default value
```

and simplifies detection of a foreign invalid state.

But source semantics MUST NOT depend on the exact numeric encoding.

#### 6.9.6 Safe enum validity invariant

In safe native Anuy:

```text
initialized value of enum E
```

always belongs to:

```text
Variants(E)
```

There is no third state:

```text
initialized but unknown discriminant
```

This is the key to sound exhaustive matching.

### 6.10 Foreign Boundary

#### 6.10.1 Go cannot enforce the closed set

A generated Go type can allow foreign Go code to create an invalid representation.

Conceptually:

```go
var c Color = Color(100)
```

if the representation is integer-like.

Such a value is a **foreign invalid state**, not a valid native Anuy value.

This is a trust-boundary problem.

#### 6.10.2 Foreign enum validation

When an enum value enters safe Anuy from an untrusted Go boundary, the implementation MUST preserve the invariant:

```text
native enum value ∈ declared variants
```

This may require validation.

The exact validation policy is defined by RFC-007/RFC-008/RFC-009.

RFC-006 fixes the semantic requirement:

> Invalid foreign discriminants must never silently become valid native enum values.

#### 6.10.3 Exhaustive match and invalid foreign values

Generated code for a match MUST remain safe even if the foreign contract is violated.

Conceptual Go lowering may contain a synthetic impossible/default branch:

```go
switch color {
case ColorRed:
    ...
case ColorGreen:
    ...
case ColorBlue:
    ...
default:
    // foreign/invariant violation
}
```

The source-level programmer does not write this branch.

It is not a semantic variant.

#### 6.10.4 Synthetic invalid branch

The synthetic default exists only to defend native invariants at backend/foreign boundaries.

It MUST:

- not count toward source exhaustiveness;
- not be user-selectable;
- map as synthetic/validation code;
- not silently continue with an arbitrary result.

The exact failure mechanism — validation error, wrapper, panic, or other contract mechanism — is defined by the interoperability RFC.

#### 6.10.5 Imported Go “enums”

The Go idiom:

```go
type State int

const (
    Ready State = iota
    Stopped
)
```

is not automatically a native Anuy enum.

The reason:

```go
State(1000)
```

remains a legal Go value.

Consequently the set of values is not closed.

#### 6.10.6 Go enum-like type remains foreign

An imported Go `type + const` API keeps Go semantics.

Anuy cannot soundly execute:

```anuy
match foreignState {
    State.Ready => ...
    State.Stopped => ...
}
```

as a native exhaustive match if arbitrary values are possible.

The programmer/library must create a native enum boundary with validation if they want a closed guarantee.

#### 6.10.7 Native adapter for Go enum-like values

Conceptually:

```anuy
type State enum {
    Ready
    Stopped
}
```

and boundary conversion:

```text
foreign Go State
    ↓ validation
native Anuy State
```

Exact conversion syntax belongs to RFC-008.

#### 6.10.8 Native enum passed to Go

A native enum MAY lower to an ordinary Go named type.

The Go consumer gets the generated representation and constants.

But the Go consumer does not get Anuy's compile-time exhaustive guarantee.

If Go later returns the enum value back, it crosses the foreign boundary again.

#### 6.10.9 Mixed `.go` + `.anuy` packages

`.go` code in a mixed package can see the generated enum representation.

Values created by `.go` source are considered foreign with respect to Anuy invariants unless the compiler/toolchain can prove otherwise.

The presence of a shared Go package MUST NOT bypass the validity guarantee.

The exact mixed-package validation strategy belongs to RFC-008/RFC-009.

### 6.11 Lowering

#### 6.11.1 Lowering `match`

Source:

```anuy
var text = match color {
    Color.Red => "red"
    Color.Green => "green"
    Color.Blue => "blue"
}
```

can conceptually be lowered to:

```go
var text string

switch color {
case ColorRed:
    text = "red"

case ColorGreen:
    text = "green"

case ColorBlue:
    text = "blue"

default:
    // synthetic invariant violation
}
```

The exact temporary strategy is a backend detail.

#### 6.11.2 Statement match lowering

Source:

```anuy
match color {
    Color.Red => {
        red()
    }

    Color.Green => {
        green()
    }

    Color.Blue => {
        blue()
    }
}
```

can use an ordinary Go `switch`.

No new runtime dispatch is needed.

#### 6.11.3 Nullable match lowering

Source:

```anuy
match color {
    nil => ...
    Color.Red => ...
    Color.Green => ...
    Color.Blue => ...
}
```

can be lowered through a representation-specific nullable test plus enum dispatch.

RFC-002 representation rules remain canonical.

RFC-006 does not require a specific nullable layout.

### 6.12 Tooling and Ecosystem

#### 6.12.1 SourceMap

The canonical semantic mapping MUST preserve:

```text
MatchID
ScrutineeSpan
ArmID
VariantID
ArmSpan
SyntheticInvalidBranch
```

Generated switch cases MUST map back to the corresponding Anuy arms.

#### 6.12.2 SourceKind

Synthetic backend code for enum matching SHOULD use:

```text
MatchDispatch
```

For foreign validity checks:

```text
Validation
```

or a more specific future refinement.

The synthetic invalid branch MUST NOT look like a user-authored default case.

#### 6.12.3 IDE support

LSP SHOULD support:

- completion of missing variants;
- add-all-missing-arms quick fix;
- navigation variant → enum declaration;
- navigation match arm → variant declaration;
- find references of variant;
- rename variant;
- enum evolution diagnostics.

#### 6.12.4 Add missing arms quick fix

Given:

```anuy
match color {
    Color.Red => {
        ...
    }
}
```

the IDE can generate:

```anuy
match color {
    Color.Red => {
        ...
    }

    Color.Green => {
        // TODO
    }

    Color.Blue => {
        // TODO
    }
}
```

Generated TODO bodies must not silently invent semantics.

Tooling may use an explicit placeholder/error until the programmer fills it in.

#### 6.12.5 Rename safety

A variant rename must use the semantic `VariantID`, not textual replacement alone.

For:

```text
Color.Red
Signal.Red
```

renaming `Color.Red` must not affect `Signal.Red`.

#### 6.12.6 Coverage

Each source match arm is a source-level branch.

The synthetic invalid/validation branch MUST NOT inflate the ordinary source coverage denominator.

CoverageMap should distinguish:

```text
User arm
Synthetic invariant branch
```

#### 6.12.7 Debugging

The debugger should step:

```text
scrutinee evaluation
→ selected source arm
```

and avoid exposing generated switch plumbing where possible.

Synthetic discriminant temporaries SHOULD be hideable.

#### 6.12.8 Reflection

Source semantics do not guarantee a particular Go numeric discriminant.

If Go reflection exposes generated underlying values, that is part of the generated-Go/interop contract, not a safe Anuy conversion mechanism.

RFC-009 defines which representation details are stable.

#### 6.12.9 Serialization

RFC-006 does not define automatic enum serialization.

The language does not silently choose between:

```text
"Red"
0
1
"red"
```

Libraries/protocol implementations must specify serialization explicitly.

This prevents declaration order from accidentally becoming the wire format.

#### 6.12.10 Database representation

Likewise no implicit DB representation exists.

The programmer/library chooses:

- string names;
- numeric codes;
- custom protocol;
- explicit adapter.

Enum source semantics remains independent.

## 7. Interaction with Other RFCs

### 7.1 RFC-001

Enum has no implicit default initialization.

```anuy
var state State
```

remains uninitialized.

Backend zero storage is not semantic enum value.

### 7.2 RFC-002

Universal nullability applies normally:

```text
State
State?
```

`State?` adds exactly `nil`, not another enum variant.

Nullable exhaustive match handles `nil` explicitly.

### 7.3 RFC-003

Enum variables use ordinary declarations and assignments:

```anuy
var state = State.Ready
state = State.Stopped
```

No special enum binding rules exist.

Match arms create ordinary lexical scopes.

### 7.4 RFC-004

Enums are named types with ordinary methods and explicit interface conformance.

```anuy
func (state State) String() string {
    ...
}

impl fmt.Stringer for State
```

### 7.5 RFC-005

Error handling remains separate from enum semantics.

RFC-006 does not model errors as enum variants.

Core errors continue to use:

```text
error?
try
```

not:

```text
Result.Ok
Result.Err
```

### 7.6 RFC-007

RFC-007 must specify what happens when foreign or unsafe code presents a discriminant outside the declared variant set.

Safe native Anuy invariant itself is defined here:

```text
every initialized enum value is valid
```

### 7.7 RFC-008

RFC-008 must define:

- imported Go enum-like types;
- validating foreign discriminants;
- passing native enums to Go;
- values returned by Go;
- mixed-package enum boundaries.

Imported Go const sets are not native enums by default.

### 7.8 RFC-009

RFC-009 must finalize:

- exact Go underlying representation;
- generated constant names;
- discriminant assignment;
- invalid-zero strategy;
- validation lowering;
- synthetic match defaults;
- ABI stability.

RFC-009 may optimize representation but cannot change the closed-set source semantics.

### 7.9 RFC-012

RFC-012, if accepted, may generalize:

```text
enum without payload
```

into richer sum types/payload variants.

It must preserve RFC-006 properties where applicable:

- closed alternatives;
- exhaustive matching;
- no silent wildcard evolution;
- sound foreign representation.

RFC-006 does not depend on RFC-012.

## 8. Diagnostics and Tooling

### 8.1 Diagnostics Catalog

The identifiers D-1…D-6 are local references within this RFC; the message forms are illustrative; stable `ANUY####` codes follow RFC-011 policy.

#### 8.1.1 D-1 — Missing Variant

```text
error: non-exhaustive match on Color

missing variants:
    Color.Blue

all variants of a native enum must be handled explicitly
```

#### 8.1.2 D-2 — Match No Longer Exhaustive After Evolution

If a dependency has added:

```text
Color.Yellow
```

the diagnostic SHOULD clearly connect the error to the changed closed set:

```text
error: match is no longer exhaustive

Color now also contains:
    Color.Yellow

add an explicit arm for Color.Yellow
```

#### 8.1.3 D-3 — Duplicate Variant

```text
error: duplicate match arm for Color.Red

this variant is already handled above
```

#### 8.1.4 D-4 — Wrong Enum

Example:

```anuy
match color {
    Signal.Red => ...
    ...
}
```

Diagnostic:

```text
error: Signal.Red cannot match Color

expected variant of:
    Color

found variant of:
    Signal
```

#### 8.1.5 D-5 — Wildcard

If the programmer writes:

```anuy
_ => ...
```

the recommended message:

```text
error: wildcard arms are not allowed in native enum matches

native enum matches must name every variant explicitly
so newly added variants produce a compile-time error

use ordinary conditional control flow if catch-all behavior is intended
```

#### 8.1.6 D-6 — Nullable Enum

```text
error: non-exhaustive match on Color?

missing:
    nil
```

or:

```text
missing:
    Color.Blue
    nil
```

as appropriate.

## 9. Rationale

### 9.1 Why enums are closed

Rejected design:

```text
packages may add variants later
```

Open variants make exhaustive compilation impossible under separate compilation.

A closed declaration gives:

- local understanding;
- exhaustiveness;
- predictable API evolution;
- compact representation;
- straightforward lowering.

## 10. Rejected Alternatives

### 10.1 Constants only

Could model:

```anuy
type Color int

const Red Color = ...
```

as Go does.

Rejected as native enum semantics because arbitrary:

```text
Color values
```

would remain possible.

The compiler then could not prove an exhaustive match.

### 10.2 Implicit default arm

Rejected:

```anuy
match color {
    Color.Red => ...
    else => ...
}
```

for native enum matching.

It defeats the requirement that adding a variant identifies all decision points needing review.

### 10.3 Automatic `Unknown`

Rejected a compiler-generated:

```text
Unknown
```

variant.

Whether unknown is meaningful belongs to domain design.

Language storage needs must not leak into the semantic variant set.

### 10.4 User-defined numeric discriminants in v1

Not introduced:

```anuy
type Status enum {
    Ready = 200
    Missing = 404
}
```

This combines:

- semantic variant identity;
- protocol constants;
- ABI representation;

too early.

Applications can explicitly map enum ↔ protocol code.

A future RFC may add representation attributes if real interoperability use cases justify them.

### 10.5 Payload enums now

Rejected for RFC-006:

```anuy
type Result enum {
    Ok(Data)
    Err(error)
}
```

This is general sum type territory.

Adding it now would require decisions about:

- payload layout;
- recursive variants;
- pattern binding;
- exhaustiveness with payload patterns;
- generics;
- Go ABI;
- zero representation;
- allocation;
- interop.

RFC-006 deliberately solves the smaller, high-value closed-enum problem first.

### 10.6 Match guards

Rejected in v1 because:

```anuy
Color.Red if condition => ...
```

does not guarantee that `Color.Red` is actually handled for all runtime states.

Ordinary nested `if` is explicit and sufficient.

### 10.7 Implicit integer enum

Enum representation may be integer-like in generated Go, but an Anuy enum is not a numeric type.

Otherwise programmers could manufacture values outside the closed set and destroy the exhaustiveness invariant.

## 12. Open Questions

No open questions are recorded in v1: all decisions are listed in Section 13; external dependencies (foreign validation, representation ABI, payload/sum types) are delegated according to Section 7 (RFC-007/008/009/012).

## 13. Normative Summary

RFC-006 v1 fixes:

1. `enum` creates a closed nominal type.
2. Enum MUST contain at least one variant.
3. Variants have no payload in v1.
4. Variant values have the enum type.
5. Canonical source reference is `Enum.Variant`.
6. Variant names are unique within an enum.
7. Distinct enum declarations create distinct types.
8. Enum has no implicit default variant.
9. `var x E` is uninitialized.
10. Enums support equality and inequality.
11. Enums do not automatically support ordering.
12. Enums are not arithmetic types.
13. No ordinary enum ↔ integer conversion exists.
14. No automatic enum ↔ string conversion exists.
15. Enums may have ordinary methods.
16. Enums may explicitly implement interfaces.
17. `match` evaluates its scrutinee exactly once.
18. Exactly one source arm executes for a valid non-null enum value.
19. Every enum variant must be explicitly handled.
20. Duplicate variant arms are errors.
21. Wildcard/default arms are absent for native enum matches in v1.
22. Match arms do not fall through.
23. Match guards are absent in v1.
24. `match` may be statement-form or value-producing.
25. Value-producing arms must form one valid result type.
26. Each block arm creates a lexical scope.
27. Exhaustive match participates in definite initialization.
28. Nullable enum match must explicitly handle `nil`.
29. `nil` arm on non-null enum is invalid.
30. Match requires a concrete statically known native enum type in v1.
31. Variant visibility follows enum visibility; externally usable enums expose their complete variant set to Anuy compilation.
32. Adding a variant is intentionally source-breaking for exhaustive consumers.
33. Removing or renaming a variant is source-breaking.
34. Source semantics do not expose numeric discriminants.
35. Safe initialized enum values can only contain declared variants.
36. Backend zero representation need not be a valid variant.
37. Compiler does not synthesize a source-visible `Unknown` variant.
38. Imported Go enum-like types are not automatically native enums.
39. Invalid foreign discriminants must not silently become native enum values.
40. Native match lowers to ordinary Go-compatible dispatch.
41. Synthetic invalid branches are not source variants.
42. Synthetic match/validation code participates in SourceMap.
43. Synthetic invalid branches do not count as user coverage branches.
44. General payload/sum types remain outside RFC-006.

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (design filter);
- RFC-001 — Values, Initialization and Trust (no default variant, zero storage);
- RFC-002 — Nullability and Nil Safety (nullable enums, `nil` arm);
- RFC-003 — Variables, Assignment and Scope (arm scopes, shadowing);
- RFC-004 — Interfaces and Explicit `impl` (enum methods/interfaces);
- RFC-005 — Error Handling and Propagation (`try` inside arms);
- RFC-007 — Unsafe and Foreign Contracts (foreign discriminants);
- RFC-008 — Go Interoperability (Go enum-like types, validation);
- RFC-009 — Lowering and Generated Go Contract (representation, ABI);
- RFC-012 — Sum Types (future generalization, status `Deferred`).
