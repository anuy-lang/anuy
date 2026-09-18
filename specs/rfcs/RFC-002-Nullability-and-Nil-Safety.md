# RFC-002 — Nullability and Nil Safety

**Status:** Accepted
**RFC:** 002
**Title:** Nullability and Nil Safety
**Language:** Anuy
**Area:** Nullability / Nil Safety / Safe Navigation / Representation / Go Interoperability
**Version:** 4
**Date:** 2026-09-19
**Requires:** RFC-000, RFC-001, RFC-003, RFC-004, RFC-005, RFC-007, RFC-008, RFC-009, RFC-011
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-002 reviewed against RFC-001 preserves the distinction between initialization and nullability; reviewed against RFC-003, it preserves refinement per binding identity.

Section 12 assigns every remaining question before `Accepted` to its responsible RFC, Language Core work or this RFC before `Accepted`.

The project owner approved RFC-002 as Proposed.

2026-09-18: the semantic core was validated by an implementation; RHS classification and establishment on initializer (§6.3.5), composite caveat (§6.1.3), strict safe-tail chains (§6.5.4), D-4 severity (§8.2.4) recorded; the owner approved the Accepted status.

2026-09-19: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 3 → 4.

---

## 1. Abstract

This RFC defines the Anuy nullability model: for any type `T` there exists a nullable type `T?` with `nil` as a distinct semantic value, non-null is the default, and nullability does not depend on the Go representation or on the presence of native `nil`. Nullability is a concept independent of initialization (RFC-001) and error propagation (`try`, RFC-005). Safe access uses flow narrowing via nil checks and safe navigation `?.` with short-circuit semantics; every nullable type has a canonical Go representation, the same for internal and exported uses.

## 2. Solutions

> **For any Anuy type `T` there exists a nullable type `T?`.**

> **uninitialized ≠ nil**

> **Error propagation does not use postfix `?` — only the `try expression`.**

> **Visibility does not change the type.**

> **Ordinary `.` on a potentially-null receiver is forbidden without proof.**

> **Nullability is determined by the meaning of the program, not by the capabilities of the Go representation.**

> **One Anuy type has one semantic identity and one canonical interoperability contract regardless of whether it is used inside a package or exported outward.**

## 3. Motivation

Absence of a value is a normal part of the business domain far beyond pointer-like values.

For example:

```text
var user User? = findUser()
var userId int? = user?.id
```

`userId` may be absent even though `int` by itself has no native Go `nil` representation.

Other natural examples:

```text
var middleName string?
var retryCount int?
var enabled bool?
var price Money?
var location Coordinates?
```

If nullable types were allowed only where the Go representation already supports `nil`, the Anuy type system would start depending on accidental properties of the backend representation.

This contradicts the architectural principle:

> **Anuy owns the semantics; Go is the backend representation.**

Therefore nullability must be determined by Anuy business semantics, not by the presence of native `nil` in Go.

## 4. Goals

The RFC defines:

- universal nullable type `T?`;
- `nil` literal;
- non-null by default;
- distinction between nullability and initialization;
- implicit widening `T → T?`;
- prohibition of implicit narrowing `T? → T`;
- control-flow narrowing;
- safe navigation `?.`;
- nullable chaining;
- nullable method calls;
- short-circuit evaluation;
- canonical Go representation;
- native nil optimization;
- tagged nullable representation;
- the same representation policy for private and public uses;
- Go interoperability;
- source mapping;
- debugging;
- coverage;
- diagnostics.

## 5. Non-goals

This RFC does not define:

- general sum types;
- nested optionality such as `Option<Option<T>>`;
- force unwrap operator;
- null-coalescing `??`;
- exact unsafe non-null assertion syntax;
- complete error handling;
- exact `try` grammar;
- foreign contract syntax;
- ownership;
- borrow checking;
- data-race prevention.

## 6. Specification

### 6.1 Core semantic model

#### 6.1.1 Core Semantic Model

For any Anuy type `T` there exists a nullable form:

```text
T?
```

Definition:

```text
Values(T?) = Values(T) ∪ {nil}
```

`nil` is not a value of type `T`.

#### 6.1.2 Non-null by Default

An ordinary type:

```text
User
```

means:

> the value is necessarily a `User`.

A nullable type:

```text
User?
```

means:

> the value is either a `User` or `nil`.

The same rule applies regardless of representation:

```text
int
int?

string
string?

*User
*User?
```

#### 6.1.3 Universal Nullability

`?` MAY be applied to any value type.

Permitted:

```text
int?
uint64?
bool?
string?
User?
Port?
[]User?
map[string]User?
*User?
Reader?
```

Nullability is not a property of the underlying Go type.

The composite spelling (`[]User?` — slice-of-nullable or
nullable-slice) is a Language Core decision (§12.2–3). Until then,
composite declared types do not participate in the flow model: deref
is not gated, establishment is not performed.

#### 6.1.4 Nullable Is a Language Type Constructor

Conceptually:

```text
Nullable(T) = T?
```

is a built-in operation of the type system.

It is not modeled as:

- compiler-known interface;
- generic user-defined type;
- pointer convention;
- Go nilability property.

#### 6.1.5 `T?` Is Not Merely Go `nil`

For:

```text
*User?
```

the compiler may use Go `nil` directly.

For:

```text
int?
```

such a representation is impossible without additional state.

The semantic meaning is the same.

#### 6.1.6 Nil Literal

`nil` is a special contextually typed literal.

Example:

```text
var user User? = nil
var count int? = nil
```

`nil` does not have its own ordinary type that is freely assignable to non-null values.

#### 6.1.7 Nil to Non-null

Not permitted:

```text
var count int = nil
var user User = nil
var ptr *User = nil
```

Compiler MUST reject such assignments.

Diagnostic — Section 8.2 (D-1).

#### 6.1.8 Nil to Nullable

Permitted:

```text
var count int? = nil
var user User? = nil
var ptr *User? = nil
```

#### 6.1.9 Nullability and Initialization

RFC-001 defines initialization separately from nullability.

Therefore:

```text
var value T
```

means:

```text
binding exists
value does not yet exist
```

And:

```text
var value T? = nil
```

means:

```text
binding exists
value exists
value == nil
```

#### 6.1.10 Uninitialized Nullable Binding

```text
var user User?
```

does not mean:

```text
user = nil
```

The binding remains uninitialized.

Before assignment:

```text
print(user)
```

MUST be a compile-time error.

#### 6.1.11 Core Distinction

Normatively:

```text
uninitialized ≠ nil
```

`uninitialized` is a compile-time state of a binding.

`nil` is a runtime semantic value of a nullable type.

#### 6.1.12 Idempotence

The nullable operation is semantically idempotent:

```text
(T?)? = T?
```

Anuy v1 does not introduce nested nullability levels.

Source syntax:

```text
int??
```

SHOULD be rejected as redundant/invalid (8.2, D-4).

#### 6.1.13 Why No Nested Nullable

`T?` models one question:

> Is there a value `T` here?

It does not model:

```text
missing
present-but-missing
present-with-T
```

If a domain requires several levels of absence, it must use an enum/sum type.

#### 6.1.14 Nullable Is Not Initialization

Using:

```text
T?
```

just to postpone initialization is discouraged and unnecessary.

RFC-001 provides:

```text
var x T
```

with definite initialization.

### 6.2 Widening, Narrowing and Nil Comparison

#### 6.2.1 Widening

For any `T` there is implicit widening:

```text
T → T?
```

Example:

```text
var id int = 42
var maybeId int? = id
```

Permitted.

#### 6.2.2 No Implicit Narrowing

The reverse conversion:

```text
T? → T
```

is not performed automatically.

Not permitted:

```text
var maybeId int? = findId()
var id int = maybeId
```

Compiler MUST require proof that the value is not `nil`.

#### 6.2.3 Nil Comparison

For a nullable value the following are allowed:

```text
value == nil
value != nil
```

#### 6.2.4 Non-null Nil Comparison

For statically non-null `T`:

```text
value == nil
```

SHOULD be a compile-time error.

Example:

```text
var user User = loadUser()

if user == nil {
    ...
}
```

Recommended diagnostic — Section 8.2 (D-5).

### 6.3 Flow Narrowing and Compiler Dataflow

#### 6.3.1 Flow Narrowing

A nil check may narrow `T?` to `T`.

Example:

```text
var user User? = findUser()

if user != nil {
    user.save()
}
```

Inside the branch:

```text
user: User
```

as a flow-refined type.

#### 6.3.2 Early Exit Narrowing

```text
var user User? = findUser()

if user == nil {
    return
}

user.save()
```

After the `if`, on the continuing path the compiler knows:

```text
user != nil
```

and treats it as `User`.

#### 6.3.3 Declared Type Does Not Change

Flow narrowing does not change the declared type.

Conceptually:

```text
declared:
    User?

current flow refinement:
    User
```

#### 6.3.4 Assignment Invalidates Narrowing

```text
if user != nil {
    user = findOtherUser()
    user.save()
}
```

If:

```text
findOtherUser() -> User?
```

the previous proof is invalidated.

`user.save()` MUST be rejected without a new check.

#### 6.3.5 Non-null Assignment Establishes Non-null State

```text
if user != nil {
    user = createUser()
    user.save()
}
```

If:

```text
createUser() -> User
```

then after the assignment `user` is flow-known non-null.

RHS classification, validated by the implementation:

- establish: non-nil literals; bindings declared non-null;
  bindings with an established flow fact; known results of calls
  with a declared non-null result (§6.4.10); untyped declarations,
  classified by the RHS (inference, platform-unknown for unresolved
  calls);
- do not establish: `nil`; nullable without a fact; calls without a
  known result; navigation; composite types (§6.1.3); closures.

Order of application: invalidation first (§6.3.4), then
establishment; the join of branches lowers facts to their
intersection. A declaration with an initializer applies the same
classification: the initializer is an ordinary assignment
(RFC-003 §13.8).

#### 6.3.6 Stable Bindings

Flow narrowing is guaranteed for bindings whose value the compiler can consider stable.

Minimally:

- local variables;
- parameters;

in the absence of hidden mutation.

#### 6.3.7 Mutable Fields

Compiler SHOULD be conservative with repeated mutable field access:

```text
if account.owner != nil {
    account.owner.save()
}
```

If `owner` may change between accesses, narrowing is not necessarily safe.

#### 6.3.8 Local Copy

The reliable form:

```text
var owner = account.owner

if owner != nil {
    owner.save()
}
```

#### 6.3.9 Captured and Escaped Bindings

Narrowing MAY be invalidated if the binding:

- captured mutable closure;
- address-taken;
- is accessible to foreign code;
- may change through an alias.

Compiler MAY apply conservative analysis.

#### 6.3.10 Compiler Dataflow

Compiler SHOULD track at least:

```text
Initialization state
+
Nullability refinement state
```

Conceptually:

```text
var user User?
```

state:

```text
Uninitialized
```

after:

```text
user = findUser()
```

state:

```text
Initialized
MaybeNil
```

after:

```text
if user == nil {
    return
}
```

continuing state:

```text
Initialized
NonNil
```

### 6.4 Safe Navigation Semantics

#### 6.4.1 Safe Navigation

Anuy v1 supports the safe navigation operator:

```text
?.
```

Example:

```text
user?.name
user?.save()
```

#### 6.4.2 Meaning of `?.`

If:

```text
receiver: T?
```

then:

```text
receiver?.member
```

means:

```text
if receiver == nil:
    result = nil
else:
    result = receiver.member
```

if the operation has a result.

#### 6.4.3 Safe Method Call

```text
user?.refresh()
```

means:

```text
if user != nil {
    user.refresh()
}
```

for a method without a result.

#### 6.4.4 Ordinary `.` on Nullable

Ordinary member access:

```text
user.name
```

on:

```text
user: User?
```

MUST be rejected if flow narrowing has not proven non-nullness.

The programmer must use:

```text
if user != nil {
    user.name
}
```

or:

```text
user?.name
```

Diagnostic — Section 8.2 (D-2).

#### 6.4.5 Safe Field Access Result

Let:

```text
user: User?
```

and:

```text
User.id: int
```

Then:

```text
user?.id
```

has type:

```text
int?
```

#### 6.4.6 Business Example

```text
var user User? = findUser()
var userId int? = user?.id
```

If:

```text
user == nil
```

then:

```text
userId == nil
```

If:

```text
user != nil
```

then:

```text
userId == user.id
```

#### 6.4.7 Nullable Member Result

Let:

```text
User.manager: User?
```

Then:

```text
user?.manager
```

has:

```text
User?
```

not a nested nullable type.

#### 6.4.8 Safe Navigation Type Rule

If the ordinary member access has result:

```text
U
```

then safe navigation has:

```text
U?
```

If the ordinary member result is already nullable:

```text
U?
```

safe navigation also returns:

```text
U?
```

#### 6.4.9 Nullable Flattening

Conceptually:

```text
lift(U):
    U?   if U is non-null
    U    if U is already nullable
```

Where in the second case `U` already has the form `X?`.

#### 6.4.10 Safe Method Result

Let:

```text
func User.age() int
```

Then:

```text
user?.age()
```

has:

```text
int?
```

#### 6.4.11 Nullable Method Result

Let:

```text
func User.manager() User?
```

Then:

```text
user?.manager()
```

has:

```text
User?
```

Method resolution is flat by name and receiver type, without
overloading; a method call with a declared result participates in the
RHS classification (§6.3.5). Purity is opt-out via the `//anuy:pure`
annotation; a call to a non-pure method invalidates narrowing of the
receiver (§6.3.9).

#### 6.4.12 Void / No-result Methods

If the method does not return a value:

```text
user?.invalidate()
```

valid as a statement.

Semantics:

```text
user == nil
    → no-op

user != nil
    → invoke invalidate()
```

No nullable void type is created.

#### 6.4.13 Short-circuit Evaluation

Safe navigation MUST short-circuit.

For example:

```text
user?.send(expensivePayload())
```

when:

```text
user == nil
```

MUST NOT evaluate:

```text
expensivePayload()
```

#### 6.4.14 Evaluation Order

For:

```text
receiver?.method(args...)
```

semantics:

1. evaluate receiver exactly once;
2. if receiver is nil, stop;
3. otherwise evaluate arguments according to ordinary order;
4. invoke method.

#### 6.4.15 Receiver Evaluated Once

Expression:

```text
findUser()?.save()
```

MUST NOT evaluate:

```text
findUser()
```

more than once.

Compiler MAY introduce synthetic temporary during lowering.

### 6.5 Chains, Typing and Formatting

#### 6.5.1 Safe Navigation Chains

Anuy supports chaining:

```text
user?.address?.city
```

Semantics short-circuits at first `nil`.

#### 6.5.2 Chain Result

If the final field is:

```text
city: string
```

then:

```text
user?.address?.city
```

has:

```text
string?
```

#### 6.5.3 No Accumulation of Nullable Levels

Chain:

```text
company?.owner?.address?.country?.code
```

does not produce:

```text
string?????
```

Result:

```text
string?
```

#### 6.5.4 Safe Navigation Through Non-null Member

If:

```text
user: User?
user.address: Address
address.city: string
```

then:

```text
user?.address.city
```

MAY be allowed as a chain after the first safe access, if the grammar and typing unambiguously establish that the successful branch has `Address`.

However, baseline syntax SHOULD prefer explicit chaining rules.

Formalization (owner decision 2026-09-14, strict safe-tail profile):
ordinary `.` is allowed only up to the first `?.`; after it only `?.`
is used, without implicit successful-branch narrowing; parentheses do
not reset the safe tail. Chains violating the profile are rejected
(`UnsupportedSyntax` until a public Language Core category is
allocated).

#### 6.5.5 Explicit Narrowing vs Safe Navigation

Both mechanisms are first-class.

Narrowing is convenient to use when the object is needed by several operations:

```text
if user == nil {
    return
}

validate(user)
save(user)
notify(user)
```

Safe navigation is convenient for an isolated optional operation:

```text
user?.notify()
```

The language does not impose a single style.

#### 6.5.6 Source Type Precedence

Postfix `?` applies to a complete type expression according to grammar.

Grammar MUST allow distinction between:

```text
map[string](User?)
```

map of nullable users

and:

```text
(map[string]User)?
```

nullable map.

#### 6.5.7 Canonical Formatting

Formatter MUST insert parentheses where necessary for unambiguous human reading.

Recommended examples:

```text
User?
*User?
int?
Reader?
(map[string]User)?
map[string](User?)
(func(Event))?
(chan Event)?
```

Exact removable-parentheses policy belongs to grammar/formatter spec.

#### 6.5.8 Safe Navigation Precedence

Expression grammar MUST distinguish:

```text
user?.manager.name
```

and chained nullable operations.

Formatter SHOULD preserve obvious reading and MAY normalize complex chains.

### 6.6 Separation of Concerns and Absent Operators

#### 6.6.1 No Force Unwrap in v1

The RFC does not introduce:

```text
user!
```

or:

```text
user!!
```

for panic/assert non-null.

Reason:

- hidden runtime failure;
- overlapping meaning `!`;
- conflict with the unsafe philosophy.

#### 6.6.2 Unsafe Non-null Assumption

If the programmer knows an invariant that the compiler is unable to prove, RFC-007 MAY define an explicit unsafe mechanism conceptually:

```text
unsafe {
    assume_non_nil(user)
}
```

This differs from a convenience force unwrap.

#### 6.6.3 Null-coalescing

Operator:

```text
??
```

is not adopted by this RFC.

For example:

```text
var id = user?.id ?? 0
```

is a natural future extension but remains a separate design question.

#### 6.6.4 Why `??` Is Deferred

`?.` is needed directly for ergonomic access to nullable values.

`??` is an additional convenience operator.

Anuy can first express fallback with ordinary control flow and evaluate the need for syntax sugar later.

#### 6.6.5 Error Propagation

Postfix `?` is not used for error propagation.

Error propagation uses:

```text
try expression
```

Example:

```text
var file = try os.Open(path)
```

#### 6.6.6 Separation of Concerns

The three mechanisms have distinct roles:

```text
var x T
    → initialization lifecycle

T?
    → value may be nil

try expr
    → propagate error
```

#### 6.6.7 `?.` Does Not Propagate Errors

Safe navigation:

```text
user?.save()
```

works only with nullable receiver semantics.

It does not replace `try`.

If `save()` itself returns error:

```text
error?
```

error handling is defined by RFC-005.

#### 6.6.8 Nullable Error

Since `error` is an ordinary Anuy type/interface type, absence of success is naturally expressed as:

```text
error?
```

For example:

```text
func Save(data Data) error?
```

#### 6.6.9 Error ABI

Go:

```go
func Save(data Data) error
```

conceptually corresponds to the Anuy:

```text
func Save(data Data) error?
```

if `nil` means success.

#### 6.6.10 Error Propagation Example

```text
func LoadConfig(path string) (Config, error?) {
    var file = try os.Open(path)
    var data = try io.ReadAll(file)
    var config = try parseConfig(data)

    return config, nil
}
```

Detailed semantics belongs to RFC-005.

#### 6.6.11 No Conflict With `try`

Type syntax:

```text
T?
```

Expression safe navigation:

```text
value?.member
```

Error propagation:

```text
try expression
```

are grammatically and conceptually distinct.

### 6.7 Canonical Representation Policy

#### 6.7.1 Canonical Representation Principle

The semantic type `T?` MUST NOT depend on:

- local vs field;
- private vs exported;
- package visibility;
- whether value crosses Go API boundary.

`T?` is a single Anuy type in all contexts.

#### 6.7.2 No Public vs Private Nullable Types

Anuy MUST NOT have concepts like:

```text
internal int?
public int?
```

with different source semantics.

Visibility does not change the type.

#### 6.7.3 Canonical Go Representation

Every Anuy type SHOULD have a canonical Go representation.

By default this representation is used:

- internally;
- across Anuy packages;
- in exported Go-facing API.

Compiler MAY internally optimize the representation only if the optimization is not observable and does not change the public ABI.

#### 6.7.4 Nullable Representation Strategy

For:

```text
T?
```

the compiler chooses the canonical representation based on the availability of free representation state.

Conceptually:

```text
if T has an available nil/niche representation:
    T? → reuse native representation
else:
    T? → tagged nullable representation
```

#### 6.7.5 Same Representation Internally and Publicly

Baseline recommendation:

> **The tagged nullable representation SHOULD be the same for internal and exported uses.**

That is, the following is not recommended:

```text
internal int? → {int,bool}
public int?   → *int
```

#### 6.7.6 Why Representation Should Not Change at Export Boundary

Different representations would create:

- adapters;
- hidden conversions;
- potential allocations;
- two ABI models;
- additional source maps;
- additional debugging complexity;
- visibility-dependent performance;
- complex generic interop semantics.

This is not justified for v1.

#### 6.7.7 Internal Optimization

Compiler MAY optimize internal nullable values:

- scalar replacement;
- register split;
- dead tag elimination;
- proven non-null specialization;

if observable semantics and ABI do not change.

#### 6.7.8 Public ABI Is a Contract

If exported:

```text
int?
```

has a canonical tagged Go representation, that representation is part of the generated Go interoperability contract.

Compiler MUST NOT change it incompatibly without a corresponding language/toolchain compatibility policy.

#### 6.7.9 Niche Optimization

Compiler MAY use a more compact native representation if it can prove the existence of an impossible bit pattern/state for non-null `T`.

However:

- semantic behavior MUST remain the same;
- the public ABI MUST follow the canonical representation contract;
- implementation-specific niche optimization MUST NOT accidentally leak into the public ABI.

#### 6.7.10 Semantic vs Representation Rule

The normative separation:

```text
Language Specification:
    T? = T or nil

Lowering Specification:
    choose efficient representation
```

Language users are not required to know whether `T?` has a tag internally.

#### 6.7.11 Compatibility

Changing:

```text
T
→
T?
```

weakens the source contract.

Callers may need new nil handling.

Changing:

```text
T?
→
T
```

strengthens return guarantees but may affect interface/function compatibility.

#### 6.7.12 Representation Compatibility

For exported types/functions, canonical Go representation of `T?` is part of interoperability contract.

Compiler-private optimizations are not.

### 6.8 Representation by Type Category

#### 6.8.1 Native Nil Representation

For example, for a non-null pointer:

```text
*User
```

valid values exclude `nil`.

Therefore:

```text
*User?
```

can use:

```go
*User
```

where:

```text
nil       → Anuy nil
non-nil   → present *User
```

#### 6.8.2 Tagged Representation

For:

```text
int?
```

all ordinary integer bit patterns are already valid `int`.

Compiler therefore needs additional presence state.

Conceptually:

```go
struct {
    value int
    present bool
}
```

#### 6.8.3 Tagged Representation Is Not Source API

The programmer does not see a type like:

```text
Nullable<int>
```

in Anuy source.

The source type remains:

```text
int?
```

The exact backing helper representation is a lowering concern.

#### 6.8.4 Pointer-like Nullable

For:

```text
*User?
```

the canonical representation can be the ordinary:

```go
*User
```

without a wrapper.

#### 6.8.5 Interfaces

If a plain Anuy interface:

```text
Reader
```

forbids a top-level nil interface value, then:

```text
Reader?
```

can use the native nil interface representation.

#### 6.8.6 Maps

If a plain:

```text
map[K]V
```

is guaranteed non-nil by Anuy semantics, then:

```text
(map[K]V)?
```

can use the ordinary Go map representation and reserve `nil` for Anuy absence.

#### 6.8.7 Channels

If a plain:

```text
chan T
```

is guaranteed non-nil, then:

```text
(chan T)?
```

can use the native nil channel representation.

#### 6.8.8 Function Values

If a plain function value is guaranteed non-nil:

```text
func(Event)
```

then the nullable form:

```text
(func(Event))?
```

can use the native Go nil function representation.

#### 6.8.9 Slices

Anuy preserves the useful Go semantics:

```text
[]T
```

MAY use a nil-backed Go slice as an ordinary valid slice value.

That is, a plain slice allows:

```go
nil
```

in the backing representation without semantic Anuy absence.

#### 6.8.10 Nullable Slice

Since the native nil representation is already taken by valid `[]T`, the nullable:

```text
[]T?
```

or canonical parsed equivalent

MUST distinguish:

```text
nil semantic value
```

from:

```text
present slice whose backing Go representation is nil
```

#### 6.8.11 Slice Nullable Representation

Therefore a nullable slice requires a tagged representation, conceptually:

```text
present=false
    → semantic nil

present=true, value=nil Go slice
    → present empty/nil-backed slice

present=true, value=non-nil Go slice
    → present slice
```

#### 6.8.12 Nil Slice Is Still Valid Plain Slice

For example:

```text
var users []User = nil
```

MAY be valid Anuy.

This means a present slice value, not nullable absence.

#### 6.8.13 Nullable Slice Example

```text
var users []User? = nil
```

if the grammar interprets `?` as the nullable whole-slice type,

means semantic absence.

After:

```text
users = []User(nil)
```

if the explicit representation/conversion syntax allows it, the value can be a present slice with nil backing.

The exact literal/conversion syntax is defined separately.

#### 6.8.14 Strings

A Go string has no nil representation.

Therefore:

```text
string?
```

uses a tagged nullable representation.

#### 6.8.15 Scalars

For example:

```text
int?
bool?
float64?
UserID?
```

usually require a tagged representation.

#### 6.8.16 Struct Values

For an ordinary:

```text
User?
```

where `User` is a value struct, the compiler usually uses a tagged representation.

### 6.9 Go Interoperability and Interfaces

#### 6.9.1 Go Interoperability

Go has no universal nullable value type.

Therefore interop depends on the canonical Go representation of the Anuy type.

#### 6.9.2 Native-nil Nullable Export

Anuy:

```text
func FindUser() *User?
```

can be exported as:

```go
func FindUser() *User
```

where nil means absence.

#### 6.9.3 Tagged Nullable Export

Anuy:

```text
func FindAge() int?
```

SHOULD use a stable canonical tagged Go representation.

Conceptually:

```go
func FindAge() anuy.Nullable[int]
```

if RFC-009 chooses a generic support type.

#### 6.9.4 No Export-specific Pointer Conversion

Baseline v1 SHOULD NOT automatically turn an exported:

```text
int?
```

into:

```go
*int
```

if the internal representation is tagged.

Reasons:

- the representation depends on visibility;
- potential allocation;
- hidden conversion;
- ABI split.

#### 6.9.5 Pure Go Consumers

The generated public nullable support representation SHOULD have a reasonable Go API.

For example, the support type MAY provide, conceptually:

```go
func Some[T any](value T) Nullable[T]
func None[T any]() Nullable[T]
func (v Nullable[T]) Get() (T, bool)
func (v Nullable[T]) IsNil() bool
```

The exact Go API is defined by RFC-008/RFC-009.

#### 6.9.6 Go-created Tagged Nullable Values

If Go can create the exported canonical nullable type, malformed representations SHOULD be structurally impossible or clearly specified.

Prefer:

```text
one presence tag
+
value payload
```

without invalid tag combinations.

#### 6.9.7 Conservative Go Imports

A Go signature does not express Anuy nullability contracts for pointer-like types.

For example:

```go
func FindUser(id int) *User
```

by default is imported as:

```text
func FindUser(id int) *User?
```

if no stronger contract exists.

#### 6.9.8 Go Scalar Returns

Go:

```go
func Age() int
```

is imported as:

```text
func Age() int
```

not `int?`, because a Go value `int` always exists.

Universal Anuy nullability does not mean conservative optionalization of all Go values.

#### 6.9.9 Go Pointer Parameter

Go:

```go
func Save(user *User)
```

does not express a prohibition of `nil`.

Therefore the imported parameter SHOULD be:

```text
*User?
```

unless the contract says otherwise.

#### 6.9.10 Go Slice Return

Go:

```go
func Users() []User
```

can be imported as the plain:

```text
[]User
```

since a nil-backed slice is a valid plain Anuy slice.

#### 6.9.11 Go Map Return

Go:

```go
func UsersByID() map[int]User
```

can return a nil map.

If a plain Anuy map is guaranteed non-nil, the conservative import must be:

```text
(map[int]User)?
```

#### 6.9.12 Go Error

Go:

```go
func Load() (Data, error)
```

imported conceptually:

```text
func Load() (Data, error?)
```

#### 6.9.13 Foreign Entry Validation

If Go calls an exported Anuy function with a non-null parameter and the Go representation can be nil, the boundary MUST NOT silently pass an invalid value into safe Anuy code.

Exact strategy:

- wrapper check;
- generated validation;
- trusted foreign contract;

is defined by RFC-008/RFC-009.

#### 6.9.14 Interface Nullability

Interface nullability refers to top-level interface absence.

Plain:

```text
Reader
```

is not a nil interface.

Nullable:

```text
Reader?
```

can be a nil interface.

#### 6.9.15 Typed Nil Interface Payload

A Go interface can be non-nil with a dynamic typed nil pointer.

For example:

```go
var p *ReaderImpl = nil
var r Reader = p
```

Top-level interface `r` non-nil.

#### 6.9.16 Typed Nil Is Not Top-level Null

Anuy interface nullability MUST distinguish:

```text
nil interface
```

from:

```text
non-nil interface containing typed nil dynamic value
```

The latter is not automatically `nil` according to `Reader?` semantics.

#### 6.9.17 Nullable Concrete to Interface

Assignment:

```text
var node *Node? = nil
var printer Printer = node
```

has potentially surprising Go semantics.

RFC-004/RFC-008 MUST definitively determine whether this boxing is:

- prohibited without narrowing;
- allowed and preserves Go typed-nil behavior;
- transformed through nullable interface.

Baseline SHOULD prefer safety and explicitness over surprising typed-nil boxing.

### 6.10 Source Mapping, Debugging and Coverage

#### 6.10.1 Source Mapping

Pure compile-time nullability checks generate no runtime spans.

User-written:

```text
if user == nil {
    return
}
```

maps normally.

#### 6.10.2 Safe Navigation Lowering

Example:

```text
var id = user?.id
```

may lower conceptually to:

```go
var id Nullable[int]

if user == nil {
    id = None[int]()
} else {
    id = Some(user.id)
}
```

Exact generated form implementation-defined.

#### 6.10.3 Single Evaluation During Lowering

For:

```text
loadUser()?.id
```

compiler MAY generate synthetic temporary:

```go
tmp := loadUser()
...
```

to guarantee receiver evaluation once.

Temporary MUST be classified synthetic.

#### 6.10.4 Safe Method Lowering

```text
user?.notify(expensive())
```

lowering MUST place argument evaluation only in non-nil branch.

#### 6.10.5 SourceMap Classification

Generated constructs for safe navigation SHOULD use:

```text
Synthetic
```

or refined kind:

```text
NullSafeNavigation
```

if SourceKind taxonomy adopts it.

User-visible source attribution remains the `?.` expression.

#### 6.10.6 Debugging

Debugger SHOULD display nullable value according to source semantics:

```text
nil
```

or present value.

It SHOULD NOT force programmer to reason about backing `present` flags unless inspecting generated Go explicitly.

#### 6.10.7 Tagged Nullable Debug Display

For:

```text
count: int?
```

debugger SHOULD show conceptually:

```text
nil
```

or:

```text
42
```

not:

```text
{value: 42, present: true}
```

in normal Anuy source debugging.

#### 6.10.8 Native-null Debug Display

For:

```text
user: *User?
```

nil pointer naturally displays as source `nil`.

#### 6.10.9 Coverage

Safe navigation expression is one user-authored operation unless its nested user expressions define additional normal coverage units.

Synthetic nil dispatch MUST NOT independently inflate coverage denominator.

#### 6.10.10 Short-circuit Coverage

For:

```text
user?.send(buildPayload())
```

coverage MAY distinguish execution of:

```text
buildPayload()
```

as its own user operation if source coverage model normally counts nested calls.

Synthetic nil check itself remains excluded.

Exact expression coverage granularity belongs to RFC-011.

### 6.11 Performance, Compatibility and Security

#### 6.11.1 Performance

Universal nullable values may have representation cost.

Native-nil nullable types can be zero-overhead:

```text
*User?
Reader?
(map[K]V)?
```

subject to semantic category.

Value types may require tag:

```text
int?
User?
string?
```

#### 6.11.2 No Hidden Allocation Requirement

Tagged `T?` MUST NOT semantically require heap allocation.

Compiler SHOULD prefer inline tagged value representation where native niche unavailable.

#### 6.11.3 Pointer-as-Optional Is Not Canonical for Value Types

Compiler SHOULD NOT define:

```text
int? → *int
```

as universal representation.

Reason:

- may introduce allocations;
- changes value semantics;
- pointer identity becomes accidental;
- performance less predictable.

#### 6.11.4 Tagged Value Semantics

Conceptually tagged nullable:

```text
present + T
```

has ordinary value semantics.

Copying `int?` copies the nullable value, not an identity-bearing box.

#### 6.11.5 Generic Nullable Support Representation

If implementation uses generic Go support:

```go
Nullable[T]
```

it SHOULD preserve value semantics and avoid mandatory allocation.

Exact definition belongs to RFC-009.

#### 6.11.6 Security Considerations

Universal null safety prevents accidental use of absent values regardless of whether `T` is pointer-like or value-like.

Examples include:

- missing IDs;
- missing counters;
- absent domain structs;
- nil pointers;
- nil callbacks;
- nil channels;
- nil maps.

#### 6.11.7 Null Safety Does Not Eliminate Other Invalid States

`int?` distinguishes:

```text
nil
```

from:

```text
0
```

but does not prove that arbitrary integer is domain-valid.

Domain invariants remain responsibility of type design.

### 6.12 Examples

#### 6.12.1 Business Scalar

```text
var user User? = findUser()
var userId int? = user?.id
```

#### 6.12.2 Optional String

```text
var middleName string? = profile?.middleName
```

No pointer wrapper is visible at source level.

#### 6.12.3 Optional Struct

```text
var address Address? = user?.address
```

Compiler uses appropriate tagged representation if `Address` has no native nullable niche.

#### 6.12.4 Narrowing

```text
var id int? = findId()

if id == nil {
    return
}

process(id)
```

Inside continuing path, `id` is flow-refined to `int`.

#### 6.12.5 Safe Method

```text
user?.notify()
```

No call occurs when `user == nil`.

#### 6.12.6 Safe Method With Argument

```text
user?.send(buildMessage())
```

`buildMessage()` is evaluated only if `user != nil`.

#### 6.12.7 Chaining

```text
var countryCode string? =
    company?.owner?.address?.country?.code
```

Result type:

```text
string?
```

#### 6.12.8 Present Slice vs Absent Slice

Conceptually:

```text
var a []int = nil
```

means:

```text
present slice value
backing representation may be nil
```

while:

```text
var b []int? = nil
```

means:

```text
semantic absence of slice value
```

These states are distinct.

#### 6.12.9 Nullable Map

```text
var cache (map[string]Value)? = loadCache()

if cache == nil {
    cache = make(map[string]Value)
}

cache["key"] = value
```

After branch merge, cache is known non-null.

#### 6.12.10 Error

```text
func Save(user User) error? {
    ...
}
```

`nil` means no error.

#### 6.12.11 Try

```text
func Load(path string) (Config, error?) {
    var data = try readFile(path)
    return parse(data)
}
```

`try` semantics belongs to RFC-005.

## 7. Interaction with Other RFCs

### 7.1 RFC-001

RFC-001 answers:

```text
Does a value exist in this binding yet?
```

RFC-002 answers:

```text
Can an existing value represent absence as nil?
```

These concerns MUST remain separate.

### 7.2 RFC-003

RFC-003 MUST define:

- assignment effects;
- scope;
- loop refinement;
- captured-variable stability;
- shadowing;
- multiple assignment.

It MUST preserve initialization/nullability analysis.

### 7.3 RFC-004

Interface RFC MUST determine:

- nullability in method signatures;
- conformance compatibility;
- nullable concrete boxing;
- typed nil behavior.

### 7.4 RFC-005

RFC-005 MUST define:

```text
try expression
```

for propagation of Go-compatible:

```text
error?
```

It MUST NOT reuse postfix `?` as propagation operator.

### 7.5 RFC-007

Unsafe RFC MAY define non-null assertion when programmer knows a fact compiler cannot prove.

Such operation MUST be explicit unsafe behavior.

### 7.6 RFC-008

Interop RFC MUST define:

- conservative import nullability;
- public tagged nullable support;
- Go construction of nullable values;
- foreign boundary validation;
- typed nil interfaces;
- Go ergonomics of `Nullable[T]`.

### 7.7 RFC-009

Lowering RFC MUST define:

- canonical representation of `T?`;
- native nil/niche reuse;
- tagged nullable representation;
- stable exported representation;
- synthetic safe-navigation lowering;
- SourceMap attribution.

### 7.8 RFC-011

Tooling RFC MUST define:

- debugger display of `T?`;
- hiding tagged backing representation;
- coverage of `?.`;
- IDE narrowing visibility;
- code actions.

## 8. Diagnostics and Tooling

### 8.1 Tooling and LSP

LSP MUST preserve source nullability.

Hover:

```text
user: User?
id: int?
```

even if generated representation is:

```go
Nullable[int]
```

or native Go pointer.

#### 8.1.1 Completion

For nullable receiver:

```text
user.
```

tooling SHOULD explain that ordinary access requires non-null proof.

For:

```text
user?.
```

completion SHOULD provide ordinary members of `User`.

#### 8.1.2 Code Actions

LSP MAY suggest:

```text
if user != nil {
    ...
}
```

or safe navigation:

```text
user?.method()
```

depending on context.

Unsafe assertion SHOULD NOT be default quick-fix.

### 8.2 Diagnostics Catalog

Identifiers D-1…D-6 are local references within the RFC; message forms are illustrative; stable `ANUY####` codes follow RFC-011 policy.

#### 8.2.1 D-1 — Nil to Non-null

```text
var id int = nil
```

Recommended:

```text
cannot use nil as int

use int? if absence is part of the value domain
```

#### 8.2.2 D-2 — Nullable Access

```text
var user User? = findUser()
user.name
```

Recommended:

```text
cannot access "name" through User?

"user" may be nil here

use a nil check or safe navigation:
    user?.name
```

#### 8.2.3 D-3 — Nullable Argument

```text
func save(user User)

var user User? = findUser()
save(user)
```

Recommended:

```text
cannot pass User? where User is required

"user" may be nil
```

#### 8.2.4 D-4 — Redundant Nullable

```text
int??
```

Recommended:

```text
int? is already nullable
```

The default severity is Warning; the code also covers redundant safe
navigation `?.` on a value known to be non-null (§12.10).
Severity configuration belongs to RFC-011.

#### 8.2.5 D-5 — Redundant Nil Check

```text
var user User = loadUser()

if user == nil {
    ...
}
```

Recommended:

```text
User is non-null and cannot be nil
```

#### 8.2.6 D-6 — Safe Navigation on Non-null

Expression:

```text
user?.name
```

where:

```text
user: User
```

SHOULD produce warning or compile-time diagnostic for redundant safe navigation.

Baseline recommendation: compile-time diagnostic.

Reason:

- stale nullability assumption;
- unnecessary control-flow syntax;
- API contract misunderstanding.

## 10. Rejected Alternatives

### 10.1 Nullable Only for Native Go Nil Types

Rejected.

This would prohibit useful types like:

```text
int?
string?
User?
```

and make language semantics depend on backend representation.

### 10.2 `Option<T>`

Rejected as primary nullability syntax.

Reasons:

- noisier;
- less familiar for simple absence;
- obscures distinction between common nullable state and future general sum types.

Anuy MAY later support general option-like enums independently.

### 10.3 Pointer Representation for All Nullable Values

Rejected.

Example:

```text
int? → *int
```

would potentially introduce:

- allocations;
- pointer identity;
- indirect access;
- unpredictable performance.

### 10.4 Different Public Representation

Rejected as baseline.

Example rejected design:

```text
internal int? → tagged
exported int? → *int
```

would make representation visibility-dependent.

### 10.5 No Safe Navigation

One could have required always writing:

```text
if user != nil {
    ...
}
```

Rejected.

For isolated optional field/method access this creates unnecessary control-flow boilerplate.

`?.` has clear local semantics and composes naturally with universal `T?`.

### 10.6 Force Unwrap

Deferred/rejected for v1.

Anuy prefers:

- flow proof;
- `?.`;
- explicit unsafe assertion where truly necessary.

### 10.7 `nullable T`

Previously considered.

Rejected in favor of:

```text
T?
```

Reasons:

- familiar notation;
- compact API signatures;
- especially natural with universal nullable value types;
- no longer conflicts with error propagation because errors use `try`.

### 10.8 Postfix `?` for Error Propagation

Rejected.

Error propagation uses:

```text
try
```

leaving:

```text
T?
```

and:

```text
?.
```

to form a coherent nullability syntax family.

## 12. Open Questions

Before `Accepted`, the following must be definitively decided:

1. exact type grammar precedence for postfix `?`;
2. canonical formatting of nullable composite types;
3. whether `[]T?` parses naturally as nullable slice or needs parentheses;
4. canonical Go support type for tagged `T?`;
5. exact public Go API of tagged nullable representation;
6. nullable concrete → non-null interface boxing;
7. typed nil interface policy;
8. narrowing of captured locals — resolved: baseline §6.3.9 and
   RFC-003 §6.5.7/§13.25–27; further nuances — Language Core during
   function design (owner decision, 2026-09-16);
9. behavior of safe navigation through mixed `?.` / `.` chains —
   resolved by owner decision 2026-09-14: strict safe-tail profile,
   §6.5.4;
10. whether redundant `?.` on non-null values is error or warning —
    resolved by owner decision 2026-09-18: Warning by default,
    §8.2.4;
11. whether `??` should be added later;
12. niche optimization policy;
13. canonical representation stability across target Go versions.

These questions do not block the Proposed status, but must be decided or kept with an explicitly assigned owner before Accepted.

- postfix-`?` precedence and composite nullable spelling → Language Core/reference manual;
- tagged nullable support type, public tagged API, niche policy and target-Go representation stability → RFC-009;
- nullable concrete-to-interface boxing and typed-nil interfaces → RFC-004 and RFC-008;
- narrowing of captured locals → RFC-003;
- mixed `?.`/`.` chaining and redundant `?.` diagnostics — resolved in this RFC (§6.5.4, §8.2.4);
- possible future `??` → future language RFC.

## 13. Normative Summary

The following are fixed normatively:

1. For any Anuy type `T` there exists a nullable type `T?`.
2. `Values(T?) = Values(T) ∪ {nil}`.
3. `T` is non-null by default.
4. `nil` is not a value of ordinary `T`.
5. `T → T?` has implicit widening.
6. `T? → T` has no implicit narrowing.
7. `(T?)?` does not create a new semantic nullable level.
8. Initialization and nullability are independent properties.
9. An uninitialized binding is not equal to an initialized `nil`.
10. Safe flow narrowing is supported via nil checks.
11. Anuy supports safe navigation `?.`.
12. Ordinary `.` on a potentially-null receiver is forbidden without proof.
13. `x?.member`, where member has `U`, produces `U?`.
14. If member already returns `U?`, safe navigation remains `U?`.
15. Safe navigation short-circuits.
16. Receiver safe navigation evaluates exactly once.
17. Method arguments are not evaluated when the receiver is nil.
18. Chaining does not create nested nullable levels.
19. The force unwrap operator is absent in v1.
20. Error propagation uses `try`, not postfix `?`.
21. `error?` is used for the ordinary nullable Go-compatible error value.
22. `T?` has a single semantics regardless of visibility.
23. Internal and exported uses are not different nullable types.
24. Every nullable type has a canonical Go representation.
25. Native nil/niche MAY be used when nil is unavailable for valid `T`.
26. If the native nil state already belongs to valid `T` or is absent, the nullable representation requires an additional tag/state.
27. `int?`, `string?`, `bool?`, struct-value `T?` are valid Anuy types.
28. The tagged nullable representation SHOULD preserve value semantics and not require heap allocation.
29. Public visibility by itself MUST NOT change the nullable representation strategy.
30. A nullable slice is semantically different from an ordinary slice even if the ordinary slice has a nil Go backing representation.
31. The generated representation must not dictate the source-level nullability model.

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (design filter);
- RFC-001 — Values, Initialization and Trust (the distinction between uninitialized and `nil`);
- RFC-003 — Variables, Assignment and Scope (assignment, scope, captured stability);
- RFC-004 — Interfaces and Explicit `impl` (nullable boxing, typed nil);
- RFC-005 — Error Handling and Propagation (`try`, `error?`);
- RFC-007 — Unsafe and Foreign Contracts (unsafe non-null assumption);
- RFC-008 — Go Interoperability (conservative imports, boundary validation);
- RFC-009 — Lowering and Generated Go Contract (canonical representation, `Nullable[T]`);
- RFC-011 — Diagnostics, Debugging and Tooling (codes, debugger display, coverage).

### 14.2 Informative
