# RFC-009 — Lowering and Generated Go Contract

**Status:** Accepted
**RFC:** 009
**Title:** Lowering and Generated Go Contract
**Language:** Anuy
**Area:** Backend / Go ABI / Representation / Source Mapping
**Version:** 6
**Date:** 2026-09-25
**Requires:** RFC-000, RFC-001, RFC-002, RFC-003, RFC-004, RFC-005, RFC-006, RFC-007, RFC-008, RFC-010, RFC-011
**Supersedes:** —

---

## Decision Record

RFC-009 was reviewed against RFC-001 (zero-backed storage is invisible until initialization), RFC-002 (nullable representation classes), RFC-005 (the error ABI) and RFC-007 (foreign entry validation). Delegations: the physical `anuyabi` module path, metadata storage, file placement — RFC-010 (Section 7.9); SourceKind refinements — RFC-011 (Section 7.10).

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

Version 5, 2026-09-24: nil-comparison dispatch and representation inference for inferred bindings. §6.7.12: `x == nil`/`x != nil` over the carrier representation lower to the carrier predicate — a verbatim struct-vs-nil comparison is not emitted. §6.7.13: a binding without a declared spelling inherits the representation of its single-call initializer's declared result — dispatch follows the representation, not the spelling (the lowering analogue of RFC-002 §6.3.11).

Version 6, 2026-09-25: published as the canonical English text of the Accepted RFC; §6.7.6–§6.7.13 and §6.8.11–§6.8.17 carry the validated contract of the ABI support package, whose physical import path is fixed in §6.7.6 (`github.com/anuy-lang/anuy/anuyabi`, same-module). No semantic changes.

---

## 1. Abstract

Anuy compiles to Go through a typed/semantic IR: the generated Go implements, but does not define, the semantics of the language. This RFC fixes the versioned Go ABI v1 — the nullable representation classes (native-nil and the tagged `anuyabi.Nullable[T]`), `uint32`-based enums with an invalid zero, ordinary Go interfaces without a vtable, the strict-error `(values..., error)` ABI, the reserved `__anuy_` namespace — and the rules of foreign entry validation. Private lowering may evolve; the public representation is stable; the canonical SourceMap is separate from the generated Go.

## 2. Solutions

> **Generated Go implements Anuy semantics; it does not define them.**

> **Anuy semantics are stronger than their Go representation, and lowering must preserve that difference without inventing a new runtime.**

> **Use ordinary Go whenever Go can represent the semantics directly; add the smallest stable representation or boundary wrapper where it cannot.**

> **Private lowering may evolve. Public representation must not drift.**

> **Strong semantics. Plain Go. Stable boundaries.**

## 3. Motivation

Lowering is where Anuy's stronger invariants are easiest to lose: it is simpler to dump nullability into Go zeros, add a hidden validity bit, or force `impl` to generate witness tables. RFC-009 fixes the opposite: the semantic IR is the only input of lowering, the representation is the minimal stable layer over ordinary Go, and the whole "strong semantics vs plain Go" difference is localized in the versioned ABI and the boundary wrappers.

## 4. Goals

RFC-009 defines:

1. the canonical Go ABI;
2. the representations of Anuy types;
3. the lowering of nullable values;
4. the enum representation;
5. interfaces and `impl`;
6. error lowering;
7. definite-initialization storage;
8. `match`;
9. safe navigation;
10. exported Go wrappers;
11. foreign-entry validation;
12. the generated-name contract;
13. optimization boundaries;
14. SourceMap requirements;
15. generated Go stability.

## 5. Non-goals

RFC-009 does not define:

- build cache layout;
- exact generated filenames;
- the CLI publish workflow;
- Go module distribution;
- the debugger UI;
- the coverage report format;
- the exact semantic-metadata serialization.

This belongs to RFC-010/RFC-011.

## 6. Specification

### 6.1 Two Contracts, ABI and Support Package

#### 6.1.1 Two contracts

The generated compiler output has two different contract layers:

```text
Anuy semantic contract
Go ABI contract
```

The Anuy semantic contract is the stronger one.

For example:

```text
Anuy *User
    guaranteed non-null

Go representation:
    *User
```

The Go type by itself is nullable, but the Anuy compiler maintains the stronger invariant.

#### 6.1.2 Generated Go is observable but not canonical source

The generated Go MAY be:

- published;
- inspected;
- built with stock Go;
- debugged;
- profiled;
- imported from ordinary Go.

But the programmer must not derive Anuy semantics from incidental details of the generated implementation.

Only the ABI properties explicitly defined by this RFC are stable.

#### 6.1.3 Go ABI v1

RFC-009 introduces the logical:

```text
Anuy Go ABI v1
```

ABI v1 fixes at minimum:

- the representation of nullable values;
- the enum underlying representation;
- exported signatures;
- interface lowering;
- the error ABI;
- public generated names;
- support representation types.

Private temporaries and lowering strategies are not part of the ABI.

#### 6.1.4 ABI changes

A change to the public canonical representation requires a new ABI version if it changes the Go type identity/layout/signature.

Breaking examples:

```text
the Nullable[T] field layout
the enum underlying type
the public generated symbol spelling
an exported function signature
```

Compiler optimizations that do not change these properties do not require an ABI bump.

#### 6.1.5 ABI support package

Some Anuy types require a representation that the standard Go type system does not directly provide.

For these, ABI v1 uses a small **ABI support package**.

In this RFC the canonical package name is:

```text
anuyabi
```

The physical Go module import path is fixed by the distribution RFC-010.

This is not an Anuy runtime.

#### 6.1.6 No Anuy runtime

`anuyabi` MUST NOT provide:

- a scheduler;
- a GC;
- an exception system;
- a dynamic dispatch registry;
- a global object model;
- a language VM;
- hidden thread state.

Anuy keeps using the Go runtime.

`anuyabi` contains only stable ABI representation helpers.

#### 6.1.7 Canonical tagged nullable

ABI v1 defines:

```go
type Nullable[T any] struct {
    Value   T
    Present bool
}
```

Conceptually:

```text
anuyabi.Nullable[T]
```

#### 6.1.8 Tagged nullable semantics

```text
Present == false
    semantic value is nil

Present == true
    semantic value is Value
```

When:

```text
Present == false
```

the field:

```text
Value
```

is ABI padding and is semantically unobservable by Anuy.

#### 6.1.9 Zero value of tagged nullable

The Go zero value:

```go
anuyabi.Nullable[T]{}
```

has:

```text
Present == false
```

and therefore correctly represents the Anuy:

```text
nil
```

This does not violate RFC-001, because a nullable type genuinely has a semantic nil value.

### 6.2 Nullable Representation

#### 6.2.1 Canonical nullable representation classes

For concrete types, ABI v1 uses two representation classes:

```text
native-nil
tagged
```

#### 6.2.2 Native-nil nullable representation

If the Go representation has a spare `nil` that is not an ordinary non-null Anuy value, `T?` MAY use that nil directly.

ABI v1 makes this canonical for:

```text
pointer types
map types
channel types
function types
interface types
error
```

Example:

```text
Anuy:
    *User?

Go:
    *User
```

#### 6.2.3 Nullable pointer

```anuy
*User?
```

lowers directly to:

```go
*User
```

Mapping:

```text
nil        → semantic nil
non-nil    → present *User
```

#### 6.2.4 Nullable map

```text
map[K]V?
```

lowers to:

```go
map[K]V
```

The Go `nil` map represents the semantic nil nullable map.

The plain non-null:

```text
map[K]V
```

uses the same physical Go type but Anuy guarantees it is non-nil.

#### 6.2.5 Nullable channel

Similarly:

```text
chan T?
```

uses Go:

```go
chan T
```

where nil means semantic nil.

#### 6.2.6 Nullable function value

```text
func(...) R?
```

when meaning a nullable function value, uses the corresponding Go function type and Go nil as semantic nil.

#### 6.2.7 Nullable interface

The native:

```text
Reader?
```

uses the ordinary generated Go:

```go
Reader
```

with a nil outer interface representing semantic nil.

The native non-null:

```text
Reader
```

uses the same Go representation but Anuy guarantees a valid non-null interface state.

#### 6.2.8 `error?`

The canonical ABI:

```text
Anuy error?
    ↔
Go error
```

A Go nil error represents the absence of an error.

This is why native fallible functions retain the ordinary Go `(T, error)` ABI.

#### 6.2.9 Tagged representation

Types without a suitable nil niche use:

```text
anuyabi.Nullable[T]
```

including:

```text
int?
bool?
string?
enum?
struct?
array?
slice?
```

where applicable.

#### 6.2.10 Nullable slices

The plain:

```text
[]T
```

uses the ordinary Go:

```go
[]T
```

including a nil-backed Go slice as a valid present slice.

Therefore the nullable:

```text
[]T?
```

MUST use:

```text
anuyabi.Nullable[[]T]
```

so that these remain distinct:

```text
semantic nil

present slice with nil Go backing
```

#### 6.2.11 No nested nullable wrapper

Source law:

```text
(T?)? = T?
```

therefore lowering never creates:

```text
Nullable[Nullable[T]]
```

merely because nullability was introduced twice.

Semantic normalization occurs before representation selection.

#### 6.2.12 Generic `T?`

Type parameters are a special ABI case.

For:

```anuy
func F[T any](value T?) ...
```

the generic Go ABI uses:

```go
anuyabi.Nullable[T]
```

regardless of the representation a concrete `T?` might otherwise use.

Reason:

> a generic declaration needs one stable Go representation before instantiation.

#### 6.2.13 Generic nullable bridges

If:

```text
T = *User
```

then the concrete:

```text
*User?
```

normally uses the native Go nil representation.

At a generic `T?` boundary the compiler inserts a representation bridge:

```text
*User?
    ↔
Nullable[*User]
```

This bridge is invisible in Anuy semantics.

#### 6.2.14 Generic ABI rule

The public generic ABI is defined by the generic declaration, not by later concrete instantiations.

The compiler MAY internally specialize generic calls, but the externally observable Go generic signature must remain stable.

### 6.3 Native Type Representations and Storage

#### 6.3.1 Primitive types

Compatible primitives lower directly:

```text
bool    → bool
string  → string
int     → int
int32   → int32
uint64  → uint64
...
```

Their Go zero values may be valid values.

RFC-001 only forbids **implicit source initialization**, not the existence of values such as:

```text
0
false
""
```

#### 6.3.2 Native pointers

The non-null Anuy:

```text
*T
```

uses the ordinary Go:

```go
*T
```

No wrapper is added.

The safe Anuy compiler ensures nil is never observed as an initialized `*T`.

Foreign entry validates non-nullness where required.

#### 6.3.3 Native slices

```text
[]T
```

uses the ordinary Go slice representation.

Nil backing is valid.

Element semantics remain Anuy semantics.

#### 6.3.4 Native arrays

```text
[N]T
```

lowers directly to a Go array where the element representation is representable.

#### 6.3.5 Native maps

The non-null:

```text
map[K]V
```

uses Go:

```go
map[K]V
```

Native Anuy guarantees the map value is not nil.

#### 6.3.6 Native channels

A non-null channel uses the ordinary Go channel representation.

Nil foreign values require boundary handling.

#### 6.3.7 Native function values

A non-null function value uses the ordinary Go function representation.

Anuy guarantees it is not nil.

#### 6.3.8 Native structs

An ordinary native aggregate lowers to an ordinary Go struct.

Conceptually:

```anuy
struct User {
    id int
    manager *User
}
```

may become:

```go
type User struct {
    id      int
    manager *User
}
```

subject to public-name projection.

#### 6.3.9 Struct zero representation

The Go zero struct MAY fail Anuy invariants.

For example, its non-null pointer fields may physically be nil.

Therefore:

```text
the Go zero representation
≠
an automatically valid native Anuy value
```

No hidden source-level default is introduced.

#### 6.3.10 No universal hidden validity bit

ABI v1 does not inject a hidden `initialized`/`valid` field into every aggregate.

Definite initialization is compiler semantics.

Foreign values are handled at boundaries through validation/contracts instead.

#### 6.3.11 Field order

The generated Go struct field order follows the canonical source field order unless another RFC explicitly defines a representation transformation.

For exported ABI-visible types, changing the field order is Go-ABI-breaking.

#### 6.3.12 Uninitialized local storage

Source:

```anuy
var connection Connection
```

may lower internally to:

```go
var connection Connection
```

if this simplifies control-flow lowering.

The physical Go zero backing remains unobservable until semantic initialization.

#### 6.3.13 Backend zero is permitted

The general rule:

> Lowering may use Go zero storage for compiler temporaries or not-yet-semantically-initialized locals.

But it MUST NOT create a safe source path capable of observing that storage.

#### 6.3.14 Package-level variables

Because native package variables require a source initializer, the generated public package state should normally be emitted with the corresponding initialization.

Lowering must preserve Go package initialization ordering and Anuy source dependency semantics.

### 6.4 Enum Representation

#### 6.4.1 Native enum representation

ABI v1 fixes native enums as named Go types based on:

```go
uint32
```

Example:

```anuy
enum Color {
    Red
    Green
    Blue
}
```

conceptually:

```go
type Color uint32

const (
    ColorRed   Color = 1
    ColorGreen Color = 2
    ColorBlue  Color = 3
)
```

#### 6.4.2 Enum discriminants

Rules:

```text
0
    reserved invalid representation

first declared variant
    1

second
    2

...

Nth
    N
```

The maximum v1 variant count:

```text
2^32 - 1
```

#### 6.4.3 Enum zero remains invalid

The generated:

```go
var color Color
```

contains the physical:

```text
0
```

but `0` is not a source variant.

Safe Anuy never observes it as an initialized enum value.

#### 6.4.4 Enum ordering is ABI-visible, not source-semantic

Source Anuy does not expose numeric discriminants.

However, the generated Go ABI does.

Therefore changing the declaration order of an exported enum is Go-ABI-breaking.

Tooling SHOULD report it accordingly.

#### 6.4.5 Enum Go constant names

The canonical public Go spelling for an exported:

```text
Enum.Variant
```

is:

```text
EnumVariant
```

Example:

```text
Color.Red
    → ColorRed
```

The compiler must deterministically escape collisions if source names would otherwise produce the same Go identifier.

### 6.5 Interfaces, Methods and Functions

#### 6.5.1 Native interfaces

The native:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}
```

lowers to the ordinary Go interface:

```go
type Reader interface {
    Read([]byte) (int, error)
}
```

subject to the representation mapping of each method type.

#### 6.5.2 No Anuy vtable

Interface calls use ordinary Go interface dispatch.

The compiler does not generate:

```text
a witness table
a custom vtable
a runtime registry
an object header
```

#### 6.5.3 `impl` lowering

Source:

```anuy
impl Reader for *File
```

is semantic metadata.

It does not generate runtime conformance state.

After checking, the compiler may erase it from the executable Go.

#### 6.5.4 No marker methods

RFC-009 preserves the RFC-004 rule:

> explicit Anuy conformance is not encoded using synthetic marker methods.

The generated method sets contain only real methods required by source/API semantics.

#### 6.5.5 Impl metadata

Although `impl` is erased from Go dispatch, the compiler metadata retains:

```text
ImplID
InterfaceType
TargetType
MethodWitnesses
SourceSpan
```

for downstream Anuy compilation and tooling.

#### 6.5.6 Method lowering

The ordinary:

```anuy
func (file *File) Read(buffer []byte) (int, error?) {
    ...
}
```

has an ordinary Go-compatible method ABI:

```go
func (file *File) Read(buffer []byte) (int, error) {
    ...
}
```

where representable.

#### 6.5.7 Exported method wrappers

An exported Go method MAY be a thin interop wrapper around an internal native implementation.

Conceptually:

```text
Go caller
    ↓
public method wrapper
    ↓ validation/projection
native method body
```

The compiler may merge the wrapper and the implementation when no boundary work is required.

#### 6.5.8 Function lowering

Anuy:

```anuy
func Add(a int, b int) int
```

normally lowers directly to:

```go
func Add(a int, b int) int
```

or to a public wrapper plus an internal implementation when required by boundary semantics.

#### 6.5.9 Native entry and Go entry

The compiler conceptually distinguishes:

```text
the native entry
the Go/foreign entry
```

Anuy-to-Anuy calls may use the native entry directly.

Ordinary external Go calls use the exported Go entry.

This distinction permits stronger validation without changing source signatures.

#### 6.5.10 Public wrapper stability

The exported Go-facing symbol is ABI-stable.

The internal native implementation symbol is not.

The compiler may rename, split, inline or remove internal functions between versions.

### 6.6 Error Lowering

#### 6.6.1 Error-only function

Anuy:

```anuy
func Save() error?
```

lowers to:

```go
func Save() error
```

#### 6.6.2 Strict fallible function

Anuy:

```anuy
func Load() (Data, error?)
```

Go ABI:

```go
func Load() (Data, error)
```

The source semantics remain RFC-005 strict:

```text
err == nil
    Data exists

err != nil
    Data is not an observable Anuy success value
```

#### 6.6.3 Failure ABI padding

For:

```anuy
return error err
```

the conceptual Go lowering:

```go
var __anuy_pad0 Data
return __anuy_pad0, err
```

The synthetic `Data` is only required by the Go calling convention.

#### 6.6.4 Failure padding is not initialization

The presence of:

```go
var __anuy_pad0 Data
```

does not introduce:

```text
Default[Data]
```

into Anuy.

Safe Anuy code cannot observe that slot on the failure path.

#### 6.6.5 Multiple success values

Anuy:

```anuy
func Parse() (Node, Meta, error?)
```

the failure lowering may require:

```go
var __anuy_pad0 Node
var __anuy_pad1 Meta
return __anuy_pad0, __anuy_pad1, err
```

#### 6.6.6 `try` lowering

Source:

```anuy
var value = try Load()
```

conceptually:

```go
__anuy_value, __anuy_err := Load()

if __anuy_err != nil {
    var __anuy_pad T
    return __anuy_pad, __anuy_err
}

value := __anuy_value
```

The actual generated form may vary.

#### 6.6.7 `try` exactly-once rule

The lowering MUST preserve:

- receiver evaluation once;
- argument evaluation once;
- the call once;
- the original evaluation order.

Compiler-generated temporaries exist precisely to preserve this.

#### 6.6.8 `return error`

`return error err` has no independent Go syntax.

The lowering materializes the failure ABI result slots and an ordinary Go `return`.

#### 6.6.9 Conditional initialization lowering

Source:

```anuy
var data, err = Load()

if err != nil {
    return error err
}

use(data)
```

may physically use normal Go locals.

The compiler relies on the semantic IR to ensure `data` is never used on an invalid path.

No runtime initialization bit is required.

### 6.7 Match and Safe Navigation Lowering

#### 6.7.1 `match` lowering

A native enum match normally lowers to an ordinary Go `switch`.

Example:

```anuy
match color {
    Color.Red => red()
    Color.Green => green()
    Color.Blue => blue()
}
```

conceptually:

```go
switch color {
case ColorRed:
    red()

case ColorGreen:
    green()

case ColorBlue:
    blue()

default:
    panic("anuy: invalid enum value")
}
```

#### 6.7.2 Synthetic enum default

The generated `default` is not a source arm.

It exists only as invariant defense.

It MUST NOT:

- satisfy source exhaustiveness;
- appear as an enum variant;
- affect exhaustive-match diagnostics.

#### 6.7.3 Invalid enum execution

Reaching the synthetic default means one of:

- a foreign contract violation;
- an unsafe contract violation;
- a compiler bug;
- corrupted state.

Fail-stop behavior may use an ordinary Go `panic`.

This panic is not language-level error propagation.

#### 6.7.4 Match expression result

Source:

```anuy
var text = match color {
    Color.Red => "red"
    Color.Green => "green"
    Color.Blue => "blue"
}
```

may lower using a zero-backed result temporary:

```go
var __anuy_result string

switch color {
...
}
```

Safe Anuy sees the result only after one valid arm assigned it.

#### 6.7.5 Safe navigation

Source:

```anuy
user?.send(expensive())
```

lowers so that:

1. the receiver evaluates once;
2. the nullable state is checked;
3. the arguments evaluate only if the receiver is present;
4. the result is converted to the correct nullable representation.

#### 6.7.6 Native-nil safe navigation

For:

```text
*User?
```

conceptually:

```go
__anuy_recv := user

if __anuy_recv != nil {
    __anuy_recv.send(expensive())
}
```

#### 6.7.7 Tagged safe navigation

For a tagged nullable:

```go
__anuy_recv := value

if __anuy_recv.Present {
    ...
}
```

Result construction uses the canonical tagged representation where required.

#### 6.7.8 Nullable flattening

For:

```text
receiver?.member
```

where the member result is already nullable, the lowering MUST preserve the source law:

```text
(T?)? = T?
```

It must not expose double tags.

#### 6.7.9 `discard`

The explicit:

```anuy
discard file.Close()
```

may lower simply to:

```go
file.Close()
```

because Go naturally permits discarded return values.

The semantic distinction exists in Anuy source/type checking, not at runtime.

#### 6.7.10 `defer discard`

```anuy
defer discard file.Close()
```

may lower directly to:

```go
defer file.Close()
```

where the Go semantics already discard returned values.

#### 6.7.11 Ordinary `defer`

Anuy defer evaluation order follows Go-compatible semantics.

The compiler MUST preserve receiver/argument capture timing.

#### 6.7.12 Nil-comparison dispatch

The exact conditions `x == nil` and `x != nil` over a binding whose representation is the tagged carrier lower to the carrier predicate:

```go
u.IsNil()      // x == nil
!u.IsNil()     // x != nil
```

Over a native-nil binding the plain Go comparison stands — there nil is the semantic nil (§6.2.2). A verbatim `== nil`/`!= nil` over a carrier-backed value MUST NOT be emitted: the struct comparison does not compile in Go, and would not be the semantic predicate if it did. The generated dispatch form is pinned by tests.

#### 6.7.13 Representation inference

A binding declared without a type spelling acquires the representation of its initializer's declared result: a single call `var u = fetch()` binds `u` under the representation class of `fetch`'s first declared result (§6.2.1) — tagged carrier or native-nil alike. Dispatch follows the representation, not the spelling — the lowering analogue of the semantic rule that refinement is independent of spelling (RFC-002 §6.3.11).

The function-result table is built from the hoisted top-level declarations before any body renders; declaration order is not observable. Unknown callees, methods and multi-result destructuring stay conservative: no dispatch registration, conditions keep the source text where Go semantics stand, and forms that demand a guard (§6.7.5) reject rather than lower unsoundly.

### 6.8 Foreign Entry Validation

#### 6.8.1 Exported Go boundary

Every exported Anuy declaration has a Go-facing contract.

External Go is allowed to manufacture values that safe Anuy itself could not.

Therefore exported entry points are foreign boundaries.

#### 6.8.2 Entry validation

Before an external Go value becomes a stronger native Anuy value, the generated entry code MUST establish the required invariants.

Examples:

```text
a non-null pointer
a valid enum discriminant
a valid tagged nullable state
native aggregate field invariants
a non-null map/channel/function
```

#### 6.8.3 Invalid Go input

If an external Go caller violates the generated Go API value contract and no source-level recovery channel exists, ABI v1 uses fail-stop boundary behavior:

```text
panic before entering the native implementation
```

This is a foreign contract violation, not an Anuy `error?`.

#### 6.8.4 Why panic at Go entry is acceptable

The generated Go API documents stronger preconditions than the raw Go type system may express.

Example Go signature:

```go
func UseUser(user *User)
```

generated from Anuy:

```text
user : *User
```

The Go caller MUST pass non-nil.

Passing nil violates the generated API contract.

The generated wrapper may detect it and panic before the native code runs.

#### 6.8.5 Non-null validation

An example conceptual wrapper:

```go
func UseUser(user *User) {
    if user == nil {
        panic("anuy: invalid foreign value: nil *User")
    }

    __anuy_useUser(user)
}
```

Anuy-to-Anuy calls may bypass this foreign-entry check.

#### 6.8.6 Enum validation

A foreign Go enum representation entering a native enum must satisfy:

```text
1 <= discriminant <= variantCount
```

for the ABI v1 contiguous enum representation.

Otherwise the boundary fails before native observation.

#### 6.8.7 Tagged nullable validation

For:

```go
Nullable[T]
```

the rules:

```text
Present == false
    Value is ignored

Present == true
    Value must itself satisfy the T invariants
```

Validation does not inspect `Value` when absent unless required for memory-safety reasons.

#### 6.8.8 Aggregate validation

If a Go-created native aggregate enters Anuy, the validator recursively validates invariant-bearing components.

Examples:

- non-null pointer fields;
- enums;
- tagged nullable present values;
- maps/channels/functions;
- arrays/slices/maps containing invariant-bearing elements;
- nested aggregates.

#### 6.8.9 Cycle-safe validation

Recursive validation MUST terminate on legal cyclic object graphs.

The generated validator may use a temporary visited-set for reference-bearing recursive types.

The exact algorithm is backend-private.

#### 6.8.10 Validation cost

Foreign-entry validation MAY be O(n) over reachable aggregate/container values when necessary.

The compiler MAY omit or simplify validation when provenance/contracts prove validity.

Correctness takes precedence over constant-time boundary projection.

#### 6.8.11 Non-null interface validation

A non-null native interface parameter or receiver entering from Go MUST reject:

- the untyped nil interface;
- a typed-nil dynamic value (`var p *File = nil; var r Reader = p`),
  per the RFC-007 §6.3.7 boundary example and the §6.9.8 rule.

Rejected states fail under §6.8.3 before native observation.

Nil-backed slices remain valid present values (RFC-007 §6.3.8); a nil
slice is never a typed-nil violation.

The implementation MAY use a direct nil comparison for concrete shapes and
Go reflection for interface shapes.

#### 6.8.12 Boundary panic value

The §6.8.3 fail-stop panic carries a canonical value defined by the ABI
support package: an error-implementing type whose message uses the
stable prefix:

```text
anuy: invalid foreign value:
```

The message identifies the offending parameter (or receiver) and the
violated invariant.

The §6.8.5 string sketch is refined, not replaced: Go hosts and tests
recover and classify the boundary panic via the value type
(`errors.As`), never by matching message text.

#### 6.8.13 Boundary coverage

The §6.8.1 foreign boundary covers:

- parameters of exported functions;
- receivers of exported methods (§6.5.7);
- both keys and values of native maps whose element types carry
  invariants.

`unsafe func` declarations are not exempt: `unsafe` relaxes source-level
obligations (RFC-007 §6.7.2), not the generated Go API contract.

A native-nil nullable parameter accepts Go nil as semantic nil (§6.2.2);
nil is never an entry violation for native-nil representation classes.

Generated unexported declarations are same-package surface; their
boundary policy belongs to RFC-008 mixed-package contracts, not §6.8.

#### 6.8.14 Native call retargeting

When an exported declaration receives a wrapper (§6.5.9), the wrapper
serves external Go callers; generated Anuy-to-Anuy calls MAY target the
internal native entry directly — the §6.8.5 bypass — as a §6.8.10 cost
measure.

Retargeting MUST NOT change observable semantics: the wrapper validates
an already-valid value identically.

#### 6.8.15 Invariant-free aggregates

An aggregate type whose transitive field closure carries no invariants
— no enum, no non-null native-nil reference, no non-null interface, no
invariant-bearing tagged payload, no invariant-bearing container
element — requires no entry validation: with nothing to establish, its
Go zero value is already a valid value.

Such aggregates are pass-through at the boundary (§6.8.10); traversal
over their fields MAY be skipped entirely.

#### 6.8.16 Per-type validators

Generated aggregate validation is factored into per-type validation
functions in the reserved namespace (§6.10.2), one per aggregate type
reachable from an exported signature through an invariant-bearing path
(§6.8.15). Entry wrappers delegate to these validators; container
parameters validate per element — keys and values (§6.8.13) — and a
present tagged payload validates through its element validator
(§6.8.7).

Validators are emitted lazily and deterministically (§13 rule 76).

#### 6.8.17 Static specialization

The compiler MAY specialize validators by static analysis of the type
graph: recursive validators carry the temporary visited-set (§6.8.9)
only when their reachable closure can contain a pointer cycle, and
provably acyclic closures may validate without it.

Specialization MUST NOT change observable behavior; which closures
require the set is backend-private.

### 6.9 Mutation, Adapters and Re-entry
#### 6.9.1 Foreign mutation

When native invariant-bearing storage is passed to foreign Go code, lowering follows RFC-008 contract.

Possible strategies:

```text
direct pass
post-call validation
copy-in/copy-out
wrapper
trusted contract
```

#### 6.9.2 Post-call validation

If foreign code may mutate native value during call but contract forbids retention, compiler MAY:

```text
validate before native reuse
```

after call return.

If contract guarantees invariant preservation, validation may be omitted.

#### 6.9.3 Retained mutable aliases

If foreign code may retain mutable reference after return and can violate native invariants, automatic safe direct projection is not permitted without an appropriate contract/wrapper.

Lowering cannot repair an open-ended invalidation channel with one check.

#### 6.9.4 Data races

Validation does not make unsynchronized concurrent mutation safe.

Foreign callers must continue to respect Go synchronization/data-race requirements.

Concurrent mutation during validation/native use is a foreign contract violation.

#### 6.9.5 Native interface from Go

Generated Go interface is structurally implementable by ordinary Go types.

When such foreign implementation enters a native Anuy interface, compiler MAY generate an interop adapter.

Conceptually:

```text
foreign Go implementation
    ↓
generated adapter
    ↓
native interface
```

#### 6.9.6 Interface adapter purpose

Adapter may:

- reject outer nil;
- reject invalid typed-nil dynamic values;
- project method arguments;
- validate method results;
- enforce foreign result contracts.

This does not create source `impl` for the foreign type.

#### 6.9.7 No runtime conformance registry

Foreign interface adaptation does not require:

- global registry;
- marker method;
- witness-table database.

Adapter existence is generated at the explicit boundary.

#### 6.9.8 Typed nil validation

A foreign Go interface entering a non-null native interface MUST NOT silently introduce a typed-nil dynamic value.

Interop wrapper must reject or adapt according to RFC-008 contract.

Implementation MAY use specialized checks or Go reflection.

#### 6.9.9 Exported interface methods

A Go implementation of generated native interface is subject to the generated Go interface contract.

If method ABI can represent invalid Anuy state, generated foreign adapter validates its results before exposing them to native code.

#### 6.9.10 Public Anuy output to Go

Values leaving native Anuy toward Go are already valid under Anuy invariants.

No validation is required merely to expose them.

Representation conversion may still be required.

#### 6.9.11 Strict error output to Go

For exported:

```anuy
func Load() (Data, error?)
```

generated Go documentation MUST state:

> If error is non-nil, preceding result values are unspecified failure padding.

Go consumers MUST NOT rely on them.

#### 6.9.12 Go re-entry

A value previously produced by Anuy but later controlled by arbitrary Go code becomes foreign on re-entry unless provenance/alias contract proves it remained valid.

Origin alone is not eternal proof under uncontrolled mutation.

#### 6.9.13 Generated public field access

Go may inspect or mutate ABI-visible generated fields where representation exposes them.

Such mutation is foreign activity.

Any subsequent upgrade to native trusted state follows entry/mutation validation rules.

### 6.10 Generated Names and Output

#### 6.10.1 Internal implementation symbols

Compiler-generated private names use reserved Go prefix:

```text
__anuy_
```

Examples:

```text
__anuy_tmp17
__anuy_pad3
__anuy_validate_User
__anuy_body_Load
```

Exact suffixes are not ABI.

#### 6.10.2 Reserved prefix in mixed packages

Handwritten `.go` files participating in an Anuy mixed package MUST NOT define or reference package identifiers beginning:

```text
__anuy_
```

Anuy tooling SHOULD diagnose such use.

This protects generated namespace.

#### 6.10.3 User source identifiers

Anuy source language need not reserve `__anuy_` merely because Go backend does.

Compiler mangling can distinguish source identity.

The restriction applies to generated Go namespace/mixed raw Go interoperability.

#### 6.10.4 Symbol identity

Lowering operates on:

```text
SymbolID
```

not textual source name.

Shadowed bindings therefore receive distinct generated identities where necessary.

#### 6.10.5 Name preservation

Compiler SHOULD preserve user-facing names when this does not create collision or semantic ambiguity.

Especially:

- exported types;
- exported functions;
- exported methods;
- exported constants.

Private local temporary spelling is not stable.

#### 6.10.6 Exported Go names

An exported Anuy declaration that already has a valid unambiguous Go identifier SHOULD preserve that identifier.

Generated helpers must not occupy user-facing exported namespace except where this RFC explicitly defines an ABI symbol.

#### 6.10.7 Generated files

Every publishable generated Go file MUST contain standard header:

```go
// Code generated by anuy. DO NOT EDIT.
```

Physical file partitioning is not semantic.

#### 6.10.8 Formatting

Published generated Go MUST be valid `gofmt`-formatted Go source.

Build-cache-only generated source SHOULD use the same formatting where practical.

#### 6.10.9 Determinism

Given identical:

```text
Anuy source
compiler version
ABI version
target Go version/configuration
```

published generated Go SHOULD be deterministically reproducible.

At minimum all ABI-visible declarations and ordering MUST be deterministic.

#### 6.10.10 Imports

Generated Go imports only packages required by lowered code/support.

Import aliases are backend-private unless part of published source readability.

Collision resolution must be deterministic.

### 6.11 SourceMap, Tooling and Optimizations

#### 6.11.1 SourceMap is canonical

Canonical mapping is not derived from generated comments.

Compiler maintains SourceMap containing at minimum:

```text
GeneratedSpan
OriginalSpan
SymbolID
SourceKind
```

#### 6.11.2 Required SourceKind categories

RFC-009 uses at least:

```text
User
Synthetic
ErrorPropagation
InteropWrapper
Validation
Temporary
MatchDispatch
NullSafeNavigation
```

Additional refinements are allowed.

#### 6.11.3 `//line`

Go:

```go
//line file.anuy:42
```

MAY be emitted to improve:

- compiler diagnostics;
- stack traces;
- debugging.

But:

> `//line` is only a projection of canonical SourceMap.

It is never the authoritative mapping database.

#### 6.11.4 Synthetic temporaries

Compiler temporaries SHOULD map to the source construct that caused them.

Example:

```text
__anuy_err7
```

generated for `try` maps to the corresponding `try` span with:

```text
SourceKind = ErrorPropagation
```

#### 6.11.5 Validation mapping

Generated foreign-boundary validator maps to the relevant exported declaration or interop conversion with:

```text
SourceKind = Validation
```

It should not appear to debugger/tooling as ordinary user-authored statement.

#### 6.11.6 Interop wrapper mapping

Generated entry/exit wrappers use:

```text
SourceKind = InteropWrapper
```

Canonical user function body retains `User` mappings.

#### 6.11.7 Match mapping

Generated switch dispatch:

```text
SourceKind = MatchDispatch
```

Individual source arm bodies map to their original source spans.

Synthetic invalid default remains synthetic.

#### 6.11.8 Coverage

Synthetic:

- ABI padding;
- validation plumbing;
- interop wrappers;
- invalid enum defaults;
- compiler temporaries

MUST NOT automatically become independent source statements in coverage denominator.

CoverageMap determines source-level accounting.

#### 6.11.9 Debugging

Generated Go must remain compatible with ordinary Go DWARF/Delve.

`//line` and canonical SourceMap should allow stepping to `.anuy` source.

Synthetic temporaries/frames MAY be hidden by Anuy DAP tooling.

#### 6.11.10 Stack traces

Whenever Go toolchain/runtime respects line directives, user stack traces SHOULD refer to Anuy source locations for user-authored code.

Generated validation failures may identify boundary wrapper location separately.

#### 6.11.11 Reflection

Ordinary Go reflection observes generated Go ABI representation.

Therefore it may see:

- `uint32`-based enum;
- `anuyabi.Nullable[T]`;
- generated Go interfaces;
- Go struct layout.

It does not automatically expose full Anuy semantic metadata.

#### 6.11.12 Reflection is not semantic authority

Anuy language semantics MUST NOT depend on Go reflection interpretation of generated representation.

Example:

```text
enum discriminant
```

is backend-visible to Go reflection but still not an ordinary Anuy integer conversion.

#### 6.11.13 Optimizations

Compiler MAY perform internal transformations such as:

- nullable tag splitting;
- scalar replacement;
- redundant-check elimination;
- wrapper elision;
- validation elimination;
- temporary elimination;
- inlining;
- constant folding.

#### 6.11.14 Nullable optimization

Internal:

```text
Nullable[int]
```

may be represented temporarily as:

```text
(int, bool)
```

or SSA values.

But at ABI boundary canonical:

```text
anuyabi.Nullable[int]
```

must be materialized.

#### 6.11.15 Native-nil optimization

Compiler may reason about nullable pointer as ordinary pointer and nil check.

It may not turn a tagged nullable slice into raw Go slice because that would collapse:

```text
semantic nil
```

and:

```text
present nil-backed slice
```

#### 6.11.16 Validation elimination

Compiler MAY eliminate boundary validation only when it has proof or trusted contract that value already satisfies native invariants.

Mere performance preference is not sufficient.

#### 6.11.17 Wrapper elision

If exported Go ABI representation and native semantics require no additional boundary action, compiler MAY merge:

```text
Go wrapper
+
native implementation
```

into one Go function.

Observable Go API must remain identical.

#### 6.11.18 Evaluation order

No optimization may change Anuy-observable evaluation order.

Especially:

- safe navigation arguments;
- function receivers;
- `try` operands;
- side-effecting initializers;
- match scrutinee.

#### 6.11.19 Panic behavior

Compiler MUST NOT introduce ordinary panics on valid safe Anuy execution except where inherited from ordinary Go semantics or separately specified.

Synthetic invariant panics occur only when a claimed invariant is violated.

### 6.13 Mixed Packages and Buildability

#### 6.13.1 Mixed package contract

Generated `.go` and handwritten `.go` sources share one Go package.

Generated declarations must therefore be accepted by stock Go package resolution.

Collisions with handwritten package declarations are compile errors.

#### 6.13.2 Calls from handwritten `.go`

Handwritten `.go` code must call public/generated Go-facing API, not `__anuy_` internal implementation symbols.

Such calls are treated as foreign entry.

#### 6.13.3 Methods added by `.go`

Go-defined methods in a mixed package participate in actual Go method set.

From Anuy semantic perspective they remain foreign methods and do not create implicit `impl`.

#### 6.13.4 Plain Go buildability

Published generated Go MUST build without Anuy compiler, using:

- supported Go toolchain;
- generated Go files;
- ordinary dependencies;
- ABI support package where required.

No compiler daemon/runtime service may be required.

#### 6.13.5 Go-only consumers

Go-only consumers receive weaker semantics.

They can:

- pass nil where Anuy would forbid it;
- construct invalid enum discriminants;
- satisfy interfaces structurally;
- inspect failure padding;
- forge nullable wrappers.

Generated API documentation/contracts define what is valid.

Anuy wrappers protect native entry where applicable.

#### 6.13.6 Anuy consumers

Anuy-to-Anuy compilation SHOULD consume semantic package metadata, not reconstruct source semantics from generated Go.

This preserves:

- explicit `impl`;
- closed enums;
- nullability;
- strict fallible contracts;
- source-level visibility;
- stronger invariants.

#### 6.13.7 ABI metadata

Each compiled/published Anuy package MUST carry enough metadata to identify:

```text
Anuy language version
Go ABI version
semantic metadata format version
```

Exact storage belongs to RFC-010.

#### 6.13.8 ABI compatibility

Compilers with compatible semantic metadata and identical Go ABI major version MAY interoperate.

Compiler MUST reject known incompatible ABI metadata rather than silently reinterpret layouts.

## 7. Interaction with Other RFCs

### 7.1 RFC-001

RFC-001 remains fully intact:

```text
uninitialized ≠ Go zero
```

RFC-009 may use zero-backed storage internally, but safe source cannot observe it before semantic initialization.

### 7.2 RFC-002

RFC-009 finalizes nullable representation:

```text
native nil
or
anuyabi.Nullable[T]
```

depending on canonical representation class.

Slices remain tagged when nullable.

### 7.3 RFC-003

Lexical bindings lower by `SymbolID`.

Shadowing is therefore backend-safe even if generated identifiers must be renamed.

`var`/assignment distinction is already resolved before lowering.

### 7.4 RFC-004

Interfaces lower to ordinary Go interfaces.

Methods lower to ordinary Go methods.

`impl` is erased from runtime Go but retained in semantic metadata.

No marker methods are emitted.

### 7.5 RFC-005

Native fallible functions preserve `(values..., error)` ABI.

`try` becomes explicit Go nil-check + early return.

Failure success-slots contain synthetic ABI padding only.

### 7.6 RFC-006

Native enums use:

```text
uint32
0 invalid
1..N declared variants
```

`match` uses Go switch plus synthetic invariant-defense default.

### 7.7 RFC-007

Unsafe does not alter canonical ABI by itself.

Foreign entry validation and trusted-contract elimination follow RFC-007 proof model:

```text
proof
validation
unsafe/trusted assertion
```

### 7.8 RFC-008

RFC-008 determines what a foreign Go value means.

RFC-009 determines how required:

- projections;
- wrappers;
- validators;
- adapters

are emitted into Go.

### 7.9 RFC-010

RFC-010 must define:

- physical `anuyabi` module path;
- generated file placement;
- build cache;
- overlays;
- package semantic metadata distribution;
- `emit-go`;
- publish workflows.

### 7.10 RFC-011

RFC-011 consumes RFC-009 SourceMap and generated-code categories for:

- diagnostics;
- gopls bridging;
- debugging;
- coverage;
- stack traces;
- synthetic-code hiding.

## 10. Rejected Alternatives

### 10.1 Syntax AST → Go AST

Rejected.

Nullability, definite initialization, explicit `impl`, error correlation and boundary validation depend on semantic facts absent from raw syntax.

Typed/Semantic IR is mandatory lowering input.

### 10.2 Universal tagged nullable

Rejected for all concrete values because it would unnecessarily destroy direct Go interoperability for:

```text
*T?
map?
chan?
func?
interface?
error?
```

ABI v1 uses native nil where it is semantically lossless.

### 10.3 Native nil for slices

Rejected because:

```text
[]T
```

already permits nil-backed present slice.

Therefore raw nil cannot also represent semantic nil for:

```text
[]T?
```

without losing a state.

### 10.4 Context-dependent public nullable ABI

For a given non-generic concrete ABI position, representation must be deterministic.

Compiler cannot randomly choose tagged vs native-nil representation per call site.

Generic `T?` is explicitly defined as a separate generic ABI case.

### 10.5 Zero enum variant

Rejected compiler-generated:

```text
Unknown = 0
```

Zero remains invalid storage representation unless programmer explicitly declares an ordinary variant whose assigned ABI discriminant is still nonzero.

### 10.6 Hidden enum default arm in semantics

Generated defensive default exists only at backend level.

It does not weaken source exhaustive matching.

### 10.7 Marker methods for `impl`

Rejected because they would alter method sets, public Go APIs and ABI.

Explicit conformance remains semantic metadata.

### 10.8 Custom interface runtime

Go interface dispatch already supplies required runtime mechanism.

A second dispatch system would violate Anuy's architecture.

### 10.9 Generated Go as semantic metadata

Generated Go cannot fully encode:

- definite initialization;
- explicit conformance intent;
- closed-world enum guarantees;
- native/foreign trust;
- strict error correlation.

Separate semantic metadata remains mandatory.

### 10.10 Unvalidated exported Go entry

External Go can construct states safe Anuy cannot.

Passing such state directly into native implementation would make language guarantees unsound.

Therefore stronger native entry requires validation or explicit contract.

### 10.11 Runtime initialized bits everywhere

Anuy definite initialization is compile-time analysis.

Injecting runtime initialized flags into every variable/aggregate would create unnecessary runtime cost and change Go ABI.

Backend zero storage is sufficient because compiler prevents invalid reads.

### 10.12 Source semantics depend on generated temporary names

Names like:

```text
__anuy_tmp7
```

are implementation detail.

Tooling uses SymbolID/SourceMap, not string matching.

## 12. Open Questions

- **OQ-1 `anuyabi` module path.** The physical Go import path of the ABI support package (Section 6.1.5) is fixed by RFC-010. — **Resolved (2026-09-25, owner decision):** `github.com/anuy-lang/anuy/anuyabi`, a same-module subpackage; recorded in RFC-010 v3 §6.7.6.
- **OQ-2 SourceKind refinements.** Extending the SourceKind taxonomy beyond the minimum of Section 6.11.2 belongs to RFC-011.

## 13. Normative Summary

RFC-009 v1 fixes:

1. Lowering input is typed/semantic IR, not syntax AST.
2. Generated Go implements but does not define Anuy semantics.
3. Anuy uses ordinary Go runtime/toolchain.
4. Anuy Go ABI is versioned.
5. ABI v1 has no custom runtime.
6. ABI support package is representation-only.
7. Tagged nullable canonical type is `anuyabi.Nullable[T]`.
8. Its fields are `Value T` then `Present bool`.
9. `Present=false` means semantic nil.
10. `Value` is semantically ignored when absent.
11. Zero tagged nullable represents nil.
12. Concrete pointer nullable uses native Go nil.
13. Concrete map nullable uses native Go nil.
14. Concrete channel nullable uses native Go nil.
15. Concrete function nullable uses native Go nil.
16. Concrete interface nullable uses native Go nil.
17. `error?` uses Go `error`.
18. Nullable slice uses tagged representation.
19. Primitive/value/aggregate nullable uses tagged representation when no nil niche exists.
20. Nested nullability is flattened semantically before lowering.
21. Generic `T?` uses tagged `Nullable[T]` ABI.
22. Compiler inserts representation bridges at generic boundaries when needed.
23. Compatible primitives lower directly.
24. Native pointers use ordinary Go pointers.
25. Native slices use ordinary Go slices.
26. Native arrays use ordinary Go arrays.
27. Native maps/channels/functions use ordinary Go representation.
28. Native structs lower to Go structs without universal hidden validity bit.
29. Go zero aggregate representation need not be valid Anuy value.
30. Native enums use named `uint32`.
31. Enum discriminant zero is invalid.
32. Enum variants receive discriminants `1..N` in declaration order.
33. Reordering exported enum variants is Go-ABI-breaking.
34. Native interfaces lower to ordinary Go interfaces.
35. Interface dispatch uses Go runtime dispatch.
36. `impl` produces no runtime witness object.
37. `impl` produces no marker method.
38. `impl` remains semantic metadata.
39. Methods preserve Go-compatible receiver ABI.
40. Exported functions/methods MAY have Go-facing wrappers.
41. Go-facing symbol is stable; internal implementation symbol is not.
42. `error?` return lowers to Go `error`.
43. Native strict fallible functions retain `(values..., error)` ABI.
44. Failure success-slots contain unspecified synthetic padding.
45. Failure padding is not Anuy initialization.
46. `try` lowers to exactly-once call, error check and early return.
47. Compiler may use zero-backed storage for uninitialized locals.
48. Such storage cannot become observable before semantic initialization.
49. Native enum `match` lowers to ordinary Go dispatch.
50. Generated invalid-enum default is synthetic only.
51. Safe navigation preserves exactly-once receiver evaluation.
52. Safe navigation short-circuits arguments.
53. `discard` may erase to ordinary Go result-discard behavior.
54. External Go entry is a foreign boundary.
55. Stronger native values must be validated/projected before native observation.
56. Immediate invalid Go input may cause boundary panic before native body.
57. Non-null values require non-nil validation when entering from Go.
58. Enums require valid-discriminant validation.
59. Present tagged nullable payloads require payload validation.
60. Native aggregate validation recursively checks invariant-bearing components.
61. Recursive validators must be cycle-safe.
62. Compiler may eliminate validation only with proof/contract.
63. Foreign mutable calls may require post-call validation or wrapper/copy strategy.
64. Retained mutable aliases require an appropriate contract.
65. Foreign Go implementations of native interfaces may use generated adapters.
66. Interface adapters do not create source `impl`.
67. Typed-nil foreign interface states must not silently enter non-null native interfaces.
68. Public output to Go is already native-valid but may require representation conversion.
69. Re-entry after uncontrolled Go mutation is foreign again.
70. Generated private symbols use reserved `__anuy_` namespace.
71. Mixed handwritten Go must not reference/define reserved generated identifiers.
72. Lowering uses SymbolID rather than textual name identity.
73. Exported user-visible Go names are preserved where possible.
74. Generated publishable Go carries standard generated-code header.
75. Published Go is `gofmt` valid.
76. ABI-visible generation is deterministic.
77. Canonical SourceMap remains separate from generated Go.
78. `//line` is only a SourceMap projection mechanism.
79. Synthetic propagation uses `ErrorPropagation`.
80. Interop wrappers use `InteropWrapper`.
81. Validators use `Validation`.
82. Match plumbing uses `MatchDispatch`.
83. Null-safe lowering uses `NullSafeNavigation`.
84. Synthetic plumbing does not automatically count as source coverage.
85. Generated Go remains compatible with normal Go debugger/runtime tooling.
86. Go reflection exposes Go ABI representation, not complete Anuy semantics.
87. Internal representation optimizations are permitted.
88. ABI boundaries must materialize canonical representation.
89. Optimizations cannot collapse semantically distinct nullable states.
90. Optimizations cannot alter evaluation order.
91. Exported entry wrappers may be elided only when semantics remain identical.
92. Generated Go must build using stock supported Go toolchain.
93. Anuy-to-Anuy consumers should use semantic metadata.
94. Generated Go alone is intentionally a weaker semantic view.
95. Package metadata carries ABI version.
96. Known incompatible ABI versions must be rejected.

RFC-009 v5 adds:

97. `x == nil`/`x != nil` over carrier-backed bindings lower to the carrier predicate; verbatim struct-to-nil comparison MUST NOT be emitted (§6.7.12).
98. A binding declared without a spelling inherits the representation of its single call initializer's declared result; unknown callees stay conservative (§6.7.13).

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (Go as the backend, no runtime);
- RFC-001 — Values, Initialization and Trust (zero-backed storage);
- RFC-002 — Nullability and Nil Safety (representation classes);
- RFC-003 — Variables, Assignment and Scope (SymbolID, shadowing);
- RFC-004 — Interfaces and Explicit `impl` (interface lowering, metadata);
- RFC-005 — Error Handling and Propagation (the strict ABI, padding);
- RFC-006 — Enums and Exhaustive Matching (the uint32 representation, the synthetic default);
- RFC-007 — Unsafe and Foreign Contracts (the entry validation model);
- RFC-008 — Go Interoperability (projection semantics, wrappers);
- RFC-010 — Build, Test and Publish Model (the `anuyabi` path, metadata storage);
- RFC-011 — Diagnostics, Debugging and Tooling (SourceKind, SourceMap consumers).
