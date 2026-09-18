# RFC-001 — Values, Initialization and Trust

**Status:** Accepted
**RFC:** 001
**Title:** Values, Initialization and Trust
**Language:** Anuy
**Area:** Values / Definite Initialization / Trust / Foreign Boundary
**Version:** 4
**Date:** 2026-09-19
**Requires:** RFC-000, RFC-002, RFC-003, RFC-005, RFC-007, RFC-008, RFC-009, RFC-010, RFC-011
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-001 reviewed against RFC-000, RFC-002 and RFC-003 preserves the distinction between uninitialized binding, initialized value and `nil`.

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or Language Core work.

The project owner approved RFC-001 as Proposed.

2026-09-19: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 3 → 4.

---

## 1. Abstract

This RFC defines the fundamental model of values and initialization in Anuy: a variable may be declared without a value; reading is permitted only after the compiler has proven initialization on every reachable control-flow path; and the uninitialized state is a compile-time state of a binding — not a value of a type and not runtime state. The RFC also defines the foundations: native and foreign values; trust boundaries; validation; interactions with Go zero representations; initialization of locals, fields and package-level variables; and source mapping, debugging and coverage for initialization.

## 2. Solutions

> **An Anuy variable may be declared without a value.**

> **Reading a variable is permitted only if the compiler has proven that it is initialized on every reachable control-flow path.**

> **The uninitialized state is a compile-time state of a binding, not a value of a type and not an observable runtime state.**

> **The Go zero value is not part of Anuy source-level initialization semantics.**

> **A nullable value and an uninitialized binding are fundamentally different states.**

> **Safe Anuy code cannot use a value of type `T` until the compiler has proven that a valid `T` actually exists.**

> **Declaration creates a binding. Initialization creates a value for that binding.**

> **`var x T` without an initializer always creates an uninitialized binding.**

## 3. Motivation

Go guarantees a zero value for any variable:

```go
var n int
var user *User
var config Config
```

After the declaration, values already exist:

```text
n      = 0
user   = nil
config = zeroed Config
```

This is a simple and convenient model, but it means that any Go type automatically has some initial state.

For Anuy this creates problems.

For example:

```text
*User
```

may be a non-null type, so `nil` is not a valid Anuy value.

The struct:

```text
Connection
```

may have no meaningful zero state.

And creating a valid value of some types may be expensive:

```text
DatabaseConnection
Cache
LargeIndex
TLSContext
```

Forcing the programmer to immediately create such values just for the sake of a declaration would be unwise.

Using nullable types as an initialization workaround is also wrong:

```text
var connection Connection? = nil
```

because that adds `nil` to the domain of the type even though the domain conceptually does not contain it.

Anuy therefore separates:

```text
declared binding
```

and:

```text
binding containing a value
```

## 4. Goals

The RFC defines:

- declaration without an initializer;
- definite initialization;
- the moment when reading a binding becomes permitted;
- the distinction between `uninitialized` and `nil`;
- initialization of locals;
- initialization of aggregate values;
- package-level initialization;
- closure capture;
- native/foreign values;
- trust transitions;
- the role of the Go zero representation;
- lowering;
- tooling semantics.

## 5. Non-goals

This RFC does not fully define:

- nullable type syntax;
- exact struct constructor syntax;
- object initialization syntax;
- field visibility;
- immutability;
- typestate;
- ownership;
- borrow checking;
- full interprocedural initialization analysis;
- lazy initialization;
- runtime `late` initialization;
- enum validity;
- foreign contract syntax.

## 6. Specification

### 6.1 Terminology

#### 6.1.1 Binding

**Binding** — a name associated with a storage/value slot in a program.

Example:

```text
var user User
```

creates the binding `user`.

A binding can be in the compile-time state:

```text
Uninitialized
```

or:

```text
Initialized(T)
```

#### 6.1.2 Value

**Value of type `T`** — a runtime value that is valid according to the semantics of type `T`.

The uninitialized state is not a value.

That is:

```text
Uninitialized
```

does not belong to the set of values of any type.

#### 6.1.3 Valid Value

**Valid value of `T`** — a value satisfying the invariants of type `T`.

Safe Anuy MUST allow reading a binding of type `T` only when the binding contains a valid `T`.

#### 6.1.4 Zero Representation

**Zero representation** — the runtime representation that backing Go storage receives under ordinary Go zero initialization.

Notation:

```text
zeroRep(T)
```

This is a backend concept.

The zero representation:

- MAY be a valid Anuy value;
- MAY be an invalid Anuy value;
- MUST NOT automatically be considered a source-level value merely because the Go storage is physically zeroed.

#### 6.1.5 Native Value

**Native value** — a value created or verified by means of safe Anuy in such a way that the compiler can consider its invariants proven.

Examples:

- literal;
- expression;
- function result from trusted Anuy code;
- validated foreign value;
- safe constructor result.

#### 6.1.6 Foreign Value

**Foreign value** — a representation/value that arrived through a boundary where the Anuy compiler cannot automatically prove all invariants.

The primary example:

```text
Go → Anuy
```

A foreign representation does not automatically become a trusted Anuy value.

### 6.2 Declaration, Initialization and Assignment

#### 6.2.1 Core Initialization Principle

The core rule:

> **Declaration creates a binding. Initialization creates a value for that binding.**

These operations are not required to happen at the same time.

#### 6.2.2 Declaration Without Initializer

The form:

```text
var x T
```

means:

> Declare a local binding `x` of type `T` without a value.

It does NOT mean:

```text
x = zeroRep(T)
```

It does NOT mean:

```text
x = nil
```

It does not invoke an implicit constructor.

#### 6.2.3 Declaration With Initializer

The forms:

```text
var x = expression
```

and:

```text
var x T = expression
```

declare and immediately initialize a binding.

After successful evaluation of the initializer, the binding is in the state:

```text
Initialized(T)
```

#### 6.2.4 Assignment

The form:

```text
x = expression
```

can perform an initial assignment or a subsequent reassignment.

If `x` was previously uninitialized:

```text
Uninitialized
↓
assignment
↓
Initialized(T)
```

If `x` is already initialized:

```text
Initialized(T)
↓
assignment
↓
Initialized(T)
```

The ordinary mutability model is defined by RFC-003.

### 6.3 Definite Initialization

#### 6.3.1 Definite Initialization

Before every read of a local binding, the compiler MUST prove:

> On every reachable control-flow path up to the given point, the binding has been initialized.

If the proof is impossible, the program MUST be rejected.

#### 6.3.2 Basic Example

Permitted:

```text
var connection Connection

connection = try openConnection()

connection.Send(data)
```

`connection` is initialized before the read.

#### 6.3.3 Conditional Initialization

Permitted:

```text
var connection Connection

if useLocal {
    connection = try openLocal()
} else {
    connection = try openRemote()
}

connection.Send(data)
```

Both branches initialize `connection`.

#### 6.3.4 Missing Initialization Path

Not permitted:

```text
var connection Connection

if useLocal {
    connection = try openLocal()
}

connection.Send(data)
```

The compiler MUST diagnose (8.2, D-1):

```text
connection may be uninitialized here
```

#### 6.3.5 Initialization Is a Control-Flow Property

The initialized/uninitialized state belongs not to the type but to the specific binding at the specific program point.

For example:

```text
var x int
```

the type of `x` is always:

```text
int
```

but the compiler state changes:

```text
before assignment:
    x: int, uninitialized

after assignment:
    x: int, initialized
```

#### 6.3.6 Locals

A local variable MAY be declared without an initializer.

The compiler MUST perform definite initialization analysis inside a function.

#### 6.3.7 Basic Blocks

For each basic block the compiler tracks the set of definitely initialized bindings.

At a merge point a binding is considered definitely initialized only if it is initialized on all incoming reachable paths.

Conceptually:

```text
InitializedAfterMerge(x)
=
AND over reachable predecessors
```

#### 6.3.8 Unreachable Paths

An unreachable control-flow path must not interfere with definite initialization.

Example:

```text
var x int

if condition {
    x = 1
} else {
    panic("stop")
}

print(x)
```

is permitted if the `panic` branch does not continue execution.

#### 6.3.9 Early Return

Permitted:

```text
var x Value

if invalid {
    return error
}

x = makeValue()
use(x)
```

The compiler analyzes only the reachable paths up to `use(x)`.

#### 6.3.10 Switch and Match

`switch` / `match` MAY definite-initialize a binding if all continuing exhaustive paths perform the assignment.

For example:

```text
var result Result

match state {
    Ready => result = makeReady()
    Failed => result = makeFailed()
}

use(result)
```

is valid with an exhaustive `match`.

#### 6.3.11 Loops

Initialization inside a loop requires careful analysis.

For example:

```text
var x int

for condition {
    x = 1
}

print(x)
```

MUST be rejected if the compiler cannot prove that the loop executes at least once (8.2, D-2).

#### 6.3.12 Infinite Loops

If the compiler can statically determine that control flow after the loop is unreachable, initialization after it is not required.

The exact loop rules are defined by RFC-003.

#### 6.3.13 Multiple Assignment

Initialization MAY happen through a multi-value assignment.

For example:

```text
var value T
var err error

value, err = load()
```

after the successful assignment both bindings are initialized.

The interaction with mandatory error handling is defined by RFC-005.

#### 6.3.14 Reassignment

After the first initialization the local binding becomes an ordinary initialized mutable variable.

For example:

```text
var count int
count = 1
count = 2
print(count)
```

is valid.

Anuy does not introduce separate syntax for the first assignment.

### 6.4 Reads, Addresses and Closures

#### 6.4.1 Read Definition

A **read** includes any use of the current value of a binding.

Examples:

```text
print(x)
foo(x)
x.field
*x
x + 1
return x
```

All of them require definite initialization.

#### 6.4.2 Assignment Target Is Not a Read

A simple assignment target:

```text
x = value
```

is not a read of `x`.

Therefore the first initialization is permitted.

#### 6.4.3 Compound Assignment

If the language supports:

```text
x += 1
```

it is simultaneously a read and a write.

Therefore `x` MUST be initialized before the compound assignment.

#### 6.4.4 Increment / Decrement

Similarly, operations of the form:

```text
x++
```

when supported, require an initialized `x`.

#### 6.4.5 Address Taking

If the language allows taking the address of a binding before initialization:

```text
&x
```

this potentially allows foreign/other code to read the backing representation.

The baseline rule:

> Address uninitialized binding MUST NOT escape.

The simplest v1 rule:

```text
&x
```

requires `x` to already be initialized.

A more complex out-parameter model may be considered separately.

#### 6.4.6 Passing as Output Storage

Anuy v1 SHOULD NOT rely on C-style out parameters for initialization.

If interop with a Go API requires pointer output arguments, a separate interop rule must guarantee that the binding becomes initialized only after a successful operation.

#### 6.4.7 Closures

Closure initialization semantics must remain simple and locally checkable.

The baseline rule:

> If a closure body reads a captured binding, that binding MUST be definitely initialized at the moment the closure is created on the path that reads it before a dominating assignment in the closure body. A capture whose first read in the body follows a dominating assignment is valid, even if the outer binding was not initialized at the point of creation. A write-only capture remains valid (Section 6.4.10, RFC-003 §6.5.5); a call of the closure never proves caller-local initialization (Section 6.4.11, RFC-003 §6.5.6).

#### 6.4.8 Invalid Closure Capture

Not permitted:

```text
var handler Handler

var callback = func() {
    handler.Handle()
}

handler = makeHandler()
```

Even though the programmer may call the callback only after the assignment, the compiler does not perform a general interprocedural call-order proof.

Diagnostic (8.2, D-3):

```text
handler is captured before it is initialized
```

#### 6.4.9 Valid Closure Capture

Permitted:

```text
var handler = makeHandler()

var callback = func() {
    handler.Handle()
}
```

#### 6.4.10 Closure Writes

A closure that only writes a binding MAY have separate rules.

For example:

```text
var x int

var init = func() {
    x = 1
}
```

However, the call:

```text
init()
print(x)
```

would require an interprocedural proof.

To keep things simple, v1 SHOULD NOT treat a call of the closure as proof of definite initialization of the outer binding.

Therefore `print(x)` in such an example MUST be rejected.

#### 6.4.11 Functions Do Not Initialize Caller Bindings Implicitly

The compiler SHOULD avoid effect analysis of the form:

```text
func initializeX()
```

as proof of local initialization.

Definite initialization must be visible directly in the control flow of the function, except for specially defined language mechanisms.

### 6.5 Aggregate Values

#### 6.5.1 Aggregate Values

An Anuy aggregate value of type:

```text
Server
```

must not become observable until all required fields are initialized.

This follows from the core invariant:

> Every observable value of type `T` is valid `T`.

#### 6.5.2 Struct Literals

A struct literal MUST initialize all required fields unless a separate RFC defines field initializer/default semantics.

Conceptually:

```text
Server {
    address: ":8080",
    handler: handler,
}
```

#### 6.5.3 Missing Field

If:

```text
struct Server {
    address string
    handler Handler
}
```

then:

```text
Server {
    address: ":8080",
}
```

MUST be rejected unless `handler` has a separate explicit initialization rule.

#### 6.5.4 No Implicit Field Zero Initialization

A missing struct field MUST NOT automatically receive the Go zero representation merely because the backing Go struct allows it.

Otherwise aggregate validity would again depend on the representation.

#### 6.5.5 Partial Aggregate Construction

This RFC admits the possibility of a compiler-internal partially initialized aggregate during construction, but such a state MUST NOT escape into ordinary safe Anuy code.

Conceptually:

```text
construction state
↓
all required fields initialized
↓
valid T
```

#### 6.5.6 Constructor Bodies

If in the future Anuy introduces a special constructor context, the compiler MAY allow staged field initialization inside it.

But the constructor MUST prove that every normal exit creates a fully initialized value.

#### 6.5.7 Returning Partially Initialized Value

MUST be a compile-time error.

For example, conceptually:

```text
var result Server
result.address = ":8080"
return result
```

if `handler` is not initialized.

#### 6.5.8 Aggregate Local Variable

The form:

```text
var server Server
```

declares an uninitialized binding `server`.

It does not create a partially initialized `Server`.

The access:

```text
server.address = ":8080"
```

requires a separate design decision about field-wise construction.

#### 6.5.9 Field-Wise Initialization of Uninitialized Aggregate

For v1, the simplest baseline:

> Fields of an uninitialized aggregate local must not be assigned before the whole value is initialized, unless there is a special construction syntax/context.

That is:

```text
var server Server
server.address = ":8080"
```

SHOULD be rejected in the baseline model.

Instead:

```text
server = Server {
    address: ":8080",
    handler: handler,
}
```

This substantially simplifies definite initialization.

#### 6.5.10 Future Construction Context

A separate RFC MAY later add controlled field-wise construction:

```text
construct Server as server {
    server.address = ...
    server.handler = ...
}
```

But this is not required for v1.

#### 6.5.11 Struct Fields

An ordinary field of an existing valid value MUST always contain a valid field value.

An uninitialized state of a normal field is not allowed after construction completes.

#### 6.5.12 No `late` Fields

Anuy v1 does not support fields that may remain uninitialized after object construction.

If the domain admits the absence of a value, the field must express that with a type:

```text
T?
```

or another explicit domain representation.

#### 6.5.13 Why No Late Fields

`late` fields would require:

- runtime initialization bits;
- runtime read checks;
- hidden state;
- aliasing semantics;
- concurrency semantics.

This significantly complicates the language model.

### 6.6 Package-Level Bindings and Parameters

#### 6.6.1 Package-Level Variables

A package-level binding MUST have an initializer.

Permitted:

```text
var config = loadConfig()
```

or:

```text
var config Config = makeConfig()
```

Not permitted:

```text
var config Config
```

Diagnostic — 8.2 (D-4).

#### 6.6.2 Why Package-Level Rule

For a package variable there is no simple intra-function control-flow proof.

Allowing uninitialized globals would require analysis of:

- package initialization ordering;
- `init`;
- function call graphs;
- callbacks;
- goroutines;
- cyclic imports/state.

Anuy avoids such complexity.

#### 6.6.3 Package Initialization May Be Expensive

The RFC does not forbid an expensive package initializer, but it is an ordinary explicit expression:

```text
var config = loadConfig()
```

The programmer sees the potential work directly in the source.

If the computation must happen later, package-level state should be modeled explicitly through a function/holder API, not through hidden lazy semantics of the language.

#### 6.6.4 Constants

Constants always have a value and do not participate in definite initialization analysis.

#### 6.6.5 Parameters

Function parameters are considered initialized on entry to the function.

If the parameter type is `T`, the caller is required to provide a valid `T` according to the ordinary type/trust rules.

#### 6.6.6 Named Results

If Anuy supports Go-like named result variables, their semantics require a separate decision.

The baseline recommendation:

> Named results SHOULD NOT be considered automatically initialized via Go zero values.

Every returned result must be definitely initialized before the return.

#### 6.6.7 Bare Return

If bare return is supported for named results, the compiler MUST prove initialization of all result bindings.

Otherwise the bare return is rejected.

#### 6.6.8 Fall-off End with Declared Result

The result of a function is a binding read by the caller: falling off
the end of the body without a return leaves it uninitialized (§13.3).
For a declared non-null result, fall-off end MUST be diagnosed as an
error (D-6, §8.2.6); for `void`/no-result functions, fall-off end is
permitted.

### 6.7 Trust and Foreign Values

#### 6.7.1 Native Values

Safe Anuy expressions create trusted native values.

Examples:

```text
42
"hello"
User { ... }
makeUser(...)
```

provided the semantics of the corresponding types is observed.

#### 6.7.2 Foreign Boundary

A foreign boundary arises when a representation comes from code not verified by Anuy semantics.

The primary example:

```text
Go function
↓
Anuy caller
```

#### 6.7.3 Foreign Is Not Uninitialized

It is important to distinguish:

```text
uninitialized binding
```

and:

```text
binding initialized foreign representation
```

The second case already has runtime bits, but their validity may not be proven.

#### 6.7.4 Trust State and Initialization State

Conceptually the compiler may track two independent things:

```text
Initialization:
    Uninitialized
    Initialized

Trust:
    Trusted
    Foreign / Unvalidated
```

However, the surface language is not required to expose a separate `Foreign<T>` type.

#### 6.7.5 Conservative Go Import

When importing a Go API, Anuy SHOULD assign a type expressing only provable guarantees.

For example:

```go
func FindUser(id int) *User
```

does not prove a non-null result.

Therefore Anuy SHOULD import it conservatively according to RFC-002.

#### 6.7.6 Go Zero Values at Boundary

Go may create:

```go
var x T
```

and pass `x` to Anuy.

If:

```text
zeroRep(T)
```

is not a valid Anuy `T`, the value cannot automatically be considered trusted.

#### 6.7.7 Trust Transition

A foreign representation becomes a trusted `T` only through a defined mechanism:

```text
static foreign contract
runtime validation
unsafe assertion
```

The exact mechanisms are defined by RFC-007 and RFC-008.

#### 6.7.8 Runtime Validation

Validation MAY check a foreign representation and, after success, return a trusted value.

Conceptually:

```text
foreign representation
↓
validate
↓
trusted T
```

Failure semantics are defined by a separate RFC.

#### 6.7.9 Unsafe Trust

Anuy MAY allow an explicit unsafe assertion:

```text
unsafe {
    assume_valid(value)
}
```

The exact syntax is defined by RFC-007.

This does not relate to initialization.

After the assignment of such a result the binding is initialized, but the correctness of the trust assumption rests with the programmer.

#### 6.7.10 Safe Assignment

An assignment to a binding of type `T` is permitted only if the RHS can be used as a valid trusted `T`.

The trust problem cannot be solved by the mere fact of assignment.

#### 6.7.11 Return Invariant

A safe Anuy function returning `T` MUST return an initialized trusted `T`.

The compiler MUST reject a path on which the result is absent or invalid.

#### 6.7.12 Mutation and Validity

After initialization, safe mutation MUST preserve the validity of the value.

If direct modification of a field can violate an invariant, a future type/visibility RFC must restrict such mutation.

#### 6.7.13 Go Aliasing

Go code can potentially violate Anuy invariants through a shared mutable representation.

Anuy v1 does not promise ownership-level guarantees.

The primary guarantee:

> Safe Anuy source itself does not create an invalid observable state.

Foreign mutation is treated as an interop/trust issue.

### 6.8 Analysis Boundaries and Dataflow Model

#### 6.8.1 Definite Initialization Is Not Typestate

Anuy does not introduce general state machine types of the form:

```text
File<Open>
File<Closed>
```

Definite initialization tracks only:

```text
value exists / value does not exist
```

for a local binding.

#### 6.8.2 Definite Initialization Is Not Null Safety

Initialization analysis answers:

> Is there a value here?

Nullability answers:

> Can an existing value be `nil`?

These are different dimensions.

#### 6.8.3 Compiler Dataflow Model

The compiler SHOULD implement definite initialization as a forward dataflow analysis.

For each binding:

```text
U = definitely uninitialized / not proven initialized
I = definitely initialized
```

Merge:

```text
I ∩ I = I
I ∩ U = U
U ∩ U = U
```

The exact internal representation is implementation-defined.

#### 6.8.4 Assignment Transfer

For:

```text
x = expr
```

after successful evaluation of the expression:

```text
state(x) = Initialized
```

If the evaluation diverges/panics/returns, there is no continuing path.

#### 6.8.5 Reads

Any read requires:

```text
state(x) = Initialized
```

otherwise a compile-time error (8.2, D-1).

#### 6.8.6 Error Propagation

Example:

```text
var file File
file = try open(path)
use(file)
```

On the success path `file` is initialized.

On the propagated-error path, execution of the function stops.

Therefore `use(file)` is valid.

#### 6.8.7 Assignment Evaluation Order

The binding becomes initialized only after successful completion of the RHS evaluation.

For example:

```text
var x T
x = try makeT()
```

on an error return `x` is not considered initialized, but continuing control flow is absent if `try` propagates.

#### 6.8.8 Self-Reference in Initializer

Not permitted:

```text
var x = x + 1
```

if this is the first declaration of the binding `x`.

The initializer does not see an initialized `x`, unless RFC-003 defines shadowing differently.

#### 6.8.9 Self-Assignment Before Initialization

Not permitted:

```text
var x int
x = x + 1
```

The RHS reads an uninitialized `x`.

#### 6.8.10 Swap / Multi-Assignment

The operation:

```text
x, y = y, x
```

requires the RHS bindings `x` and `y` to be initialized before the operation.

If one of them is uninitialized — a compile-time error.

### 6.9 Uninitialized, `nil` and Zero Representation

#### 6.9.1 Uninitialized Is Not Nullable

The key distinction:

```text
uninitialized ≠ nil
```

Example:

```text
var user *User
```

means:

> `user` does not yet contain a value.

Whereas:

```text
var user *User? = nil
```

means:

> `user` contains a valid nullable value `nil`.

#### 6.9.2 Nullable Does Not Solve Initialization

The programmer SHOULD NOT be forced to write:

```text
var connection Connection? = nil
```

merely because `connection` will become available later.

If `nil` is not a domain state, the type should remain:

```text
Connection
```

and the initialization lifecycle is expressed by definite initialization analysis.

#### 6.9.3 No `late`

Anuy v1 does not introduce `late`.

The reason:

`var x T` + definite initialization already solves the main use case of deferred initialization without:

- runtime checks;
- hidden initialization flags;
- a nullable workaround;
- a separate language feature.

#### 6.9.4 No Lazy Initialization Semantics

The form:

```text
var x T
```

is not a lazy initializer.

It only declares a binding.

If an expensive value is needed only later, the programmer explicitly assigns it at the needed control-flow location.

For example:

```text
var index Index

if needSearch {
    index = buildIndex()
    runSearch(index)
}
```

#### 6.9.5 No Hidden Runtime Initialization Check

For ordinary locals Anuy MUST NOT insert a runtime check:

```text
if !initialized {
    panic(...)
}
```

Definite initialization is a compile-time property.

If the compiler cannot prove initialization, the program is rejected.

#### 6.9.6 No Runtime Uninitialized Value

A runtime value:

```text
Uninitialized<T>
```

does not exist.

Anuy does not introduce:

- sentinel value;
- hidden wrapper;
- hidden optional;
- runtime tag;

for ordinary local initialization.

#### 6.9.7 Go Lowering of Uninitialized Locals

Generated Go MAY physically use:

```go
var x T
```

before the first assignment.

This is permitted because the compiler guarantees that source Anuy cannot read the backing zero representation before initialization.

Thus:

```text
Go storage may exist
```

does not mean:

```text
Anuy value exists
```

#### 6.9.8 Backend Zero Representation

Example:

```text
var connection Connection
```

may lower to:

```go
var connection Connection
```

Go physically creates:

```text
zeroRep(Connection)
```

But before the first assignment this representation MUST be considered inaccessible to the Anuy program.

It is a storage implementation detail.

#### 6.9.9 Zero Representation and Validity

Both cases are possible:

```text
zeroRep(T) ∈ Valid(T)
```

and:

```text
zeroRep(T) ∉ Valid(T)
```

Initialization semantics does not change because of this.

For example:

```text
var n int
print(n)
```

MUST be a compile-time error, even though `zeroRep(int) == 0` is a valid integer.

If the programmer needs `0`, they write:

```text
var n = 0
```

#### 6.9.10 Why No Implicit Zero Read

If an implicit zero read were allowed for only some types, the language would have to explain:

```text
var n int       // initialized?
var p *User     // uninitialized?
var s string    // initialized?
var cfg Config  // depends?
```

This creates type-dependent declaration semantics.

Anuy prefers a single rule:

> **`var x T` without an initializer always creates an uninitialized binding.**

#### 6.9.11 Uniform Declaration Semantics

Therefore:

```text
var x int
var y string
var z bool
var u User
var p *User
```

have the same semantics:

```text
binding declared
value absent
```

The type does not change the meaning of the declaration.

#### 6.9.12 Explicit Zero Initialization

If the programmer wants a zero-like value, they must write it explicitly:

```text
var count = 0
var enabled = false
var name = ""
```

For an aggregate, the corresponding explicit constructor/literal may be used.

### 6.10 Source Mapping, Debugging and Coverage

#### 6.10.1 Source Mapping

A declaration without an initializer:

```text
var x T
```

is a user-visible declaration, but not an executable initialization operation.

Compiler-generated backing storage MAY have a generated span; however, it SHOULD be classified as a storage/synthetic implementation detail.

#### 6.10.2 Assignment Source Mapping

The first assignment:

```text
x = makeT()
```

is an ordinary user operation and receives ordinary SourceMap attribution.

There is no separate hidden default construction.

#### 6.10.3 Debugging

The debugger SHOULD show a local variable only where it is technically and semantically reasonable.

Before initialization the backing Go slot MAY exist in DWARF, but its physical zero contents are not an Anuy value.

#### 6.10.4 Debugger Presentation Before Initialization

A polished Anuy DAP layer MAY:

- hide an uninitialized local;
- show `<uninitialized>`;

but MUST NOT show the backing zero representation as if it were a valid source value.

For example, it should not show:

```text
connection = Connection{}
```

if the source binding is still uninitialized.

#### 6.10.5 Breakpoints and Initialization

Stepping through:

```text
var x T
x = makeT()
```

must reflect the source operations.

A declaration without an executable initializer MAY lack a separate runtime stop.

The assignment is a runtime operation.

#### 6.10.6 Coverage

The declaration:

```text
var x T
```

without an initializer SHOULD NOT be a standalone executable coverage unit.

The assignment:

```text
x = makeT()
```

is a coverage unit according to the general rules.

#### 6.10.7 Declaration With Initializer Coverage

```text
var x = makeT()
```

is a single source-level executable operation.

Generated storage/declaration mechanics do not create separate coverage units.

### 6.11 Compatibility and Security

#### 6.11.1 Relationship to Go

Anuy deliberately differs from Go.

Go:

```go
var x int
fmt.Println(x) // 0
```

Anuy:

```text
var x int
print(x) // compile-time error
```

For Anuy this is an intentional safety/readability trade-off.

#### 6.11.2 Go Interoperability

Generated Go MAY use zero-initialized storage, but the Anuy semantic layer MUST NOT expose it as a value before assignment.

This means:

```text
Go zero value
```

remains part of the backend implementation and the foreign boundary model, but not of user initialization semantics.

#### 6.11.3 Public Generated API

This RFC does not require changing the ordinary Go ABI rules.

If Anuy exports a Go-visible type `T`, a Go consumer may still be able to write:

```go
var x T
```

What happens when such a value is returned back to Anuy is determined by the trust/interop rules.

#### 6.11.4 Compatibility

The main difference from Go:

```text
var x T
```

does not give a readable zero value.

Migration Go → Anuy may require adding an explicit initializer:

```text
var count = 0
```

or moving the assignment closer to the real point of initialization.

#### 6.11.5 Security Considerations

Definite initialization prevents:

- accidental use of the backing zero representation;
- accidental nil use through zeroed non-null storage;
- reading a logically invalid aggregate state;
- some of the bugs related to a missing assignment.

The RFC does not prevent:

- data races;
- unsafe memory corruption;
- malicious foreign code;
- post-initialization invariant violations through uncontrolled mutation.

#### 6.11.6 Conformance Tests

A minimal test suite:

```text
local declaration without initializer
read before initialization
direct initialization
if/else initialization
missing branch
early return
panic/diverging branch
loop may execute zero times
exhaustive match initialization
multi-assignment
self-read during first assignment
closure capture before initialization
package variable without initializer
non-null pointer declaration
nullable nil value
generated Go zero storage isolation
debug uninitialized presentation
coverage behavior
```

### 6.12 Examples

#### 6.12.1 Expensive Type

```text
var connection Connection

if request.requiresDatabase {
    connection = try openConnection()
    process(connection)
}
```

No `Connection` is created unless needed.

No nullable wrapper required.

#### 6.12.2 Primitive

```text
var count int
count = calculateCount()
print(count)
```

Valid.

But:

```text
var count int
print(count)
```

compile-time error.

#### 6.12.3 Explicit Zero

```text
var count = 0
print(count)
```

Valid.

Zero is explicit programmer intent.

#### 6.12.4 Nullable

```text
var user *User? = nil

if found {
    user = loadUser()
}
```

`user` is initialized from declaration.

Its value MAY be `nil` because type permits it.

#### 6.12.5 Non-null Deferred Initialization

```text
var user *User

if cached {
    user = cachedUser()
} else {
    user = try loadUser()
}

use(user)
```

No `nil` state is introduced.

#### 6.12.6 Invalid Non-null Use

```text
var user *User

if cached {
    user = cachedUser()
}

use(user)
```

Rejected because `user` may be uninitialized.

#### 6.12.7 Package Variable

Invalid:

```text
var config Config
```

Valid:

```text
var config = loadConfig()
```

#### 6.12.8 Closure

Invalid:

```text
var service Service

var handler = func() {
    service.Run()
}

service = createService()
```

Valid:

```text
var service = createService()

var handler = func() {
    service.Run()
}
```

#### 6.12.9 Loop

Invalid:

```text
var value int

for hasItems() {
    value = next()
}

print(value)
```

Compiler cannot prove loop executed.

#### 6.12.10 Early Exit

Valid:

```text
var value Value

if !ready {
    return
}

value = createValue()
use(value)
```

#### 6.12.11 Match

```text
var message string

match state {
    Ready => message = "ready"
    Failed => message = "failed"
}

print(message)
```

Valid if `match` exhaustive and both continuing arms assign.

## 7. Interaction with Other RFCs

RFC-001 defines the initialization dimension and delegates the adjacent dimensions to other RFCs:

- **RFC-002** — nullability and conservative import of the Go API (Sections 6.7.5, 6.9.1–6.9.2): `uninitialized` and `nil` are different states;
- **RFC-003** — the assignment/reassignment model, mutability, scope, shadowing and detailed loop definite-assignment rules (Sections 6.2.4, 6.3.12, 6.4.10);
- **RFC-005** — the interaction of initialization with mandatory error handling (Section 6.3.13);
- **RFC-007 / RFC-008** — trust transition mechanisms: static foreign contracts, validation, unsafe assertion, address-taking interop (Sections 6.7.7–6.7.9, 6.4.5–6.4.6);
- **RFC-009** — lowering of uninitialized locals into generated Go (Sections 6.9.7–6.9.8);
- **RFC-010** — package initialization for tests/generated code (Section 12);
- **RFC-011** — diagnostics, debugger presentation, the declaration-distance warning (Sections 8, 12).

## 8. Diagnostics and Tooling

### 8.1 Tooling Requirements

The compiler/LSP MUST support:

- definite initialization diagnostics;
- control-flow explanation;
- initialization state for editor analysis;
- closure-capture diagnostics;
- package initializer diagnostics.

Debugger tooling SHOULD distinguish an uninitialized binding from the backing zero representation.

### 8.2 Diagnostics Catalog

The identifiers D-1…D-5 are local references inside the RFC; the message forms are illustrative; stable codes `ANUY####` follow RFC-011 policy.

#### 8.2.1 D-1 — Read Before Initialization

The primary diagnostic:

```text
variable "x" may be uninitialized here
```

The compiler SHOULD indicate:

- the declaration;
- the missing control-flow path;
- the relevant branch, if possible.

Recommended extended form:

```text
connection may be uninitialized here

"connection" is initialized only when "useLocal" is true
```

#### 8.2.2 D-2 — Loop May Execute Zero Times

```text
x may be uninitialized here

the loop may execute zero times
```

#### 8.2.3 D-3 — Capture Before Initialization

```text
handler is captured before it is initialized
```

#### 8.2.4 D-4 — Package-Level Variable Requires Initializer

```text
package-level variable "config" requires an initializer
```

#### 8.2.5 D-5 — No `Default` Suggestion

Since `Default` does not exist in the initialization model, the compiler MUST NOT suggest:

```text
implement Default
```

as a solution to the initialization error.

The solution is an explicit assignment/initializer.

#### 8.2.6 D-6 — Missing Return

```text
missing return: declared result is not initialized on all paths
```

A function with a declared non-null result falls off the end of the
body (§6.6.8). Severity: Error. References: Go "missing return",
C# CS0161, TypeScript TS2355, Kotlin "missing return statement".

## 9. Rationale

### 9.1 Benefits of Explicit Initialization

Explicit initialization provides:

- the same semantics for all types;
- no special `Default`;
- no default vs zero distinction;
- no hidden expensive construction;
- no forced nullable;
- simpler control-flow reasoning;
- more understandable diagnostics.

### 9.2 Benefits for Coverage

The absence of an implicit `Default` means that coverage does not need to account for hidden initialization code.

Source-level execution stays closer to what the programmer explicitly wrote.

### 9.3 Benefits for Debugging

The absence of implicit constructors means that the line:

```text
var x T
```

hides no arbitrary computation.

This improves the predictability of stepping.

### 9.4 Benefits for Performance Reasoning

A declaration without an initializer has no source-level construction cost.

If creating `T` is expensive, the cost appears only where the programmer explicitly writes the construction:

```text
x = expensiveCreate()
```

### 9.5 Benefits for Language Simplicity

The model does not require:

- `Default`;
- a compiler-known initialization interface;
- `late`;
- implicit nullable initialization;
- a zero/default value distinction;
- implicit constructors.

The core rule is uniform for all types.

### 9.6 Migration Benefit

Migration Go → Anuy, which requires an explicit initializer, also uncovers cases where Go code implicitly relied on a zero state.

This matches the goals of Anuy:

> programmer intent becomes explicit.

## 10. Rejected Alternatives

### 10.1 Implicit Go Zero Values

One could have kept the Go semantics:

```text
var x T
```

automatically creates `zeroRep(T)`.

Rejected.

Reasons:

- invalid zero states;
- implicit nullable values;
- type-dependent validity;
- makes expensive semantic initialization a separate problem;
- weakens the stronger-invariant goal.

### 10.2 `Default`

One could have defined a special contract:

```text
Default
```

and treated:

```text
var x T
```

as `T.default()`.

Rejected.

Reasons:

- a special compiler-known concept just for the sake of initialization;
- a declaration can hide arbitrary construction cost;
- creates a zero/default distinction;
- complicates the mental model;
- requires solving side effects/failure/purity of `Default`;
- not needed with definite initialization.

### 10.3 Universal `Default` Protocol Family

One could have created a general system of compiler-known interfaces and included `Default` in it.

Deferred/rejected for this problem.

Compiler-known protocols should appear only if there exists an independent group of language features that genuinely requires a common mechanism.

Initialization by itself does not justify such an abstraction.

### 10.4 `late`

One could have added:

```text
late var x T
```

with runtime initialization checking.

Rejected for v1.

Definite initialization already solves the main use case without runtime state.

### 10.5 Nullable as Deferred Initialization

One could write:

```text
var x T? = nil
```

and later assign a `T`.

Rejected as an initialization strategy.

Nullable must mean a real domain state, not a lifecycle workaround.

### 10.6 Automatic Lazy Initialization

One could have made:

```text
lazy var x = expensive()
```

or a Dart-like lazy `late`.

Deferred.

Concurrency semantics for goroutines requires a separate design:

- exactly once;
- synchronization;
- panic behavior;
- recursive access;
- memory ordering.

This problem does not belong to the base initialization semantics.

### 10.7 Runtime Initialized Bit for All Bindings

One could store a hidden boolean next to every potentially-uninitialized value.

Rejected.

Definite initialization is solved at compile time and requires no runtime overhead.

### 10.8 Full Interprocedural Analysis

The compiler could prove:

```text
initialize()
use(x)
```

if the function `initialize` is guaranteed to assign the captured/global variable.

Rejected for baseline v1.

Such analysis complicates reasoning and language implementation.

The initialization proof SHOULD be local and obvious.

## 12. Open Questions

Before `Accepted`, the following must be finally decided:

1. exact field-wise construction rules;
2. whether a special construction context for aggregates is allowed;
3. detailed loop definite-assignment rules;
4. exact closure capture rule for write-only captures — resolved by owner
   decision 2026-09-15: Section 6.4.7 fixes the capture dataflow rule (a first
   read after the dominating assignment in the body is valid; a write-only
   capture remains valid — RFC-003 §6.5.5; a call of the closure does not
   prove caller initialization — RFC-003 §6.5.6);
5. address-taking/out-parameter interop;
6. named result variables;
7. package initialization restrictions for tests/generated code;
8. debugger presentation of uninitialized locals;
9. the interaction of definite initialization with future immutable bindings;
10. whether compiler warnings should flag declaration far before first assignment.

These questions do not block the Proposed status, but must be resolved or kept with an explicitly assigned owner before `Accepted`.

- field-wise construction and the special aggregate construction context → Language Core/reference manual;
- detailed loop definite-assignment and write-only closure capture → RFC-003;
- address-taking/out-parameter interop → RFC-008;
- named results → Language Core/reference manual;
- package initialization for tests/generated code → RFC-010;
- debugger presentation → RFC-011;
- immutable bindings → future immutability RFC;
- declaration-distance warning → RFC-011.

## 13. Normative Summary

The following are fixed normatively:

1. `var x T` without an initializer declares an uninitialized local binding.
2. `var x T` does not create a zero/default value.
3. Reading a binding requires definite initialization.
4. Initialization must be proven on all reachable paths.
5. The uninitialized state is not a runtime value.
6. Uninitialized and `nil` are different concepts.
7. A nullable type is not used as an implicit initialization mechanism.
8. Anuy v1 has no `Default`.
9. Anuy v1 has no `late`.
10. Anuy v1 has no implicit lazy initialization.
11. Ordinary local definite initialization requires no runtime flag/check.
12. The Go zero representation is a backend/foreign detail.
13. Package-level variables require an initializer.
14. Ordinary aggregate values must be fully initialized before observation.
15. The compiler MUST NOT pass off the backing zero representation as an Anuy value.
16. Foreign trust and initialization are separate dimensions.
17. Debugger/coverage/source mapping must reflect the explicit Anuy initialization semantics.
18. A declared non-null result MUST be initialized on all exit paths (D-6).

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (design filter);
- RFC-002 — Nullability and Nil Safety (the distinction between `nil` and uninitialized, conservative import);
- RFC-003 — Variables, Assignment and Scope (assignment model, loop rules, closure capture §83–84);
- RFC-005 — Error Handling and Propagation (multi-assignment with `error?`);
- RFC-007 — Unsafe and Foreign Contracts (unsafe trust assertion);
- RFC-008 — Go Interoperability (foreign boundary, address-taking interop);
- RFC-009 — Lowering and Generated Go Contract (zero-initialized storage);
- RFC-010 — Build, Test and Publish Model (package initialization for tests);
- RFC-011 — Diagnostics, Debugging and Tooling (diagnostic codes, debugger presentation).

### 14.2 Informative
