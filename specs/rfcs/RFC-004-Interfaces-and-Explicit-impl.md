# RFC-004 — Interfaces and Explicit `impl`

**Status:** Accepted
**RFC:** 004
**Title:** Interfaces and Explicit `impl`
**Language:** Anuy
**Area:** Type System / Interfaces / Methods / Go Interoperability
**Version:** 5
**Date:** 2026-09-24
**Requires:** RFC-000, RFC-001, RFC-002, RFC-007, RFC-008, RFC-009, RFC-011
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-004 was reviewed against RFC-000 (rejection of metaprogramming and runtime extension mechanisms), RFC-002 (interface nullability and nullable impl targets, Section 6.4) and RFC-003 (method receivers as ordinary bindings in lexical scope).

Delegated questions: typed-nil boxing and conversion of a nullable concrete to an interface — jointly with RFC-008 (see RFC-002, Section 6.9.17 and Section 12); the export metadata format — RFC-009/RFC-011.

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

2026-09-24: receiver binding specified (§6.1.7, new subsection appended): the receiver is an ordinary parameter of the method body — RFC-003 scope and RFC-002 flow rules apply verbatim; receiver spellings are `T`, `*T` and `*T?` (the nullable value form `T?` is not a receiver — the generated Go cannot define methods on the carrier type); named, unnamed and blank Go forms are valid; method-set semantics unchanged (owner decision, 2026-09-24); Version 3 → 4.

2026-09-24: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 4 → 5.

---

## 1. Abstract

Anuy keeps Go interfaces as the primary model of runtime polymorphism, but replaces implicit structural conformance for native Anuy types with an explicit declaration of intent: `impl Reader for *File` declares that the compiler MUST prove conformance, while methods belong to the type independently of interfaces. `impl` has no body and creates no runtime mechanisms — after type checking it is semantic metadata; the runtime representation remains an ordinary Go interface. Conformance is determined via Go method sets, coherence is ensured by the target-type ownership rule, and nullable interfaces follow RFC-002 without exceptions.

## 2. Solutions

> **Methods are structural capabilities; interface conformance is explicit intent.**

> **Nominal intent, structural verification, Go representation.**

> **Structural completeness does not create conformance by itself.**

> **Conformance authority belongs to the concrete type owner.**

> **Explicit conformance is an Anuy source-language invariant, not a claim that arbitrary Go code cannot construct structurally compatible values.**

> **Anuy keeps Go’s method and interface machinery, but makes the decision to conform an explicit, checked part of the program.**

> **Structural methods. Explicit contracts. Go dispatch.**

## 3. Motivation

In Go, a method whose signature matches an interface method automatically makes the type an implementation of that interface. Such accidental matching is impossible to distinguish from an intentional API commitment: refactoring or adding an interface retroactively silently changes the set of types that "implement" it, and the place where the commitment was accepted is not recorded in the code.

Anuy makes the commitment explicit and checkable: conformance is declared at a specific point of the program, diagnostics on interface change are localized to that point, and the runtime representation remains the ordinary Go model.

## 4. Goals

RFC-004 MUST provide:

1. explicit interface conformance;
2. preservation of the familiar Go method sets;
3. absence of accidental conformance;
4. localized diagnostics on interface change;
5. proper support for pointer/value receivers;
6. the ability of one method to satisfy several interfaces;
7. coherence across packages;
8. generic interfaces and generic impl;
9. direct lowering to Go interfaces;
10. predictable interoperability with Go.

The key design principle:

> **Nominal intent, structural verification, Go representation.**

Interface identity and the fact of conformance are explicit in Anuy.

But the compiler verifies conformance via the method set, and the runtime representation remains a structural Go interface.

## 5. Non-goals

RFC-004 does not introduce:

- class inheritance;
- virtual methods outside Go interfaces;
- default interface methods;
- trait method bodies;
- specialization;
- negative impl;
- a runtime reflection registry for `impl`;
- implicit orphan implementations;
- extension methods on foreign types;
- its own dispatch model.

Dynamic type assertions, exhaustive matching on the dynamic type, and the details of foreign contract validation are defined by separate RFCs.

## 6. Specification

### 6.1 Interfaces and Methods

#### 6.1.1 Interface declarations

A native Anuy interface is declared:

```anuy
interface Reader {
    Read(buffer []byte) (int, error?)
}
```

Several methods:

```anuy
interface ReadWriter {
    Read(buffer []byte) (int, error?)
    Write(buffer []byte) (int, error?)
}
```

An interface declaration establishes the nominal interface identity.

Two interfaces with identical method sets are not one Anuy interface:

```anuy
interface ReaderA {
    Read([]byte) (int, error?)
}

interface ReaderB {
    Read([]byte) (int, error?)
}
```

`ReaderA` and `ReaderB` are distinct interface types.

A concrete type MAY implement both, but conformance is declared separately.

#### 6.1.2 Method declarations

Methods stay as close to Go as possible.

```anuy
func (file *File) Read(buffer []byte) (int, error?) {
    ...
}
```

Value receiver:

```anuy
func (file File) Name() string {
    return file.name
}
```

RFC-004 deliberately does **not** introduce:

```anuy
impl File {
    ...
}
```

for declaring ordinary methods.

Reasons:

- Go receiver syntax already expresses receiver identity precisely;
- pointer/value semantics are obvious;
- lowering is almost direct;
- a method declaration is not mixed with interface conformance;
- one method can participate in several interfaces;
- there is no problem of "which interface owns the method".

#### 6.1.3 Explicit conformance

For a native Anuy interface, structural matching is not enough.

```anuy
func (file *File) Read(buffer []byte) (int, error?) {
    ...
}
```

by itself does **not mean** that `*File` implements `Reader`.

Required:

```anuy
impl Reader for *File
```

The compiler verifies the declaration at the `impl` point.

If the method set does not match the interface, the program is invalid.

Example:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}

func (file *File) Read(buffer []byte) (int, error?) {
    ...
}

impl Reader for *File
```

is valid.

Without the last line:

```anuy
var file = openFile()

var reader Reader = file
```

is invalid even when the method set matches structurally.

#### 6.1.4 `impl` has no body

The v1 syntax:

```anuy
impl Reader for *File
```

not:

```anuy
impl Reader for *File {
    func Read(...) {
        ...
    }
}
```

and not:

```anuy
impl Reader for *File {
    Read
}
```

Interface methods are implemented by ordinary methods of the type.

`impl` only asserts conformance.

This is intentional.

This design rules out the situation where one type receives different implementations of the same method through different interfaces.

For example:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}

interface Source {
    Read([]byte) (int, error?)
}
```

One method:

```anuy
func (file *File) Read(buffer []byte) (int, error?) {
    ...
}
```

can serve both declarations:

```anuy
impl Reader for *File
impl Source for *File
```

There is no method duplication.

#### 6.1.5 Direct method calls do not require `impl`

`impl` controls interface conformance, not the presence of methods.

```anuy
var file File = ...
file.Name()
```

is valid if `File` has the corresponding method.

No interface declaration is required for a direct method call.

That is:

```text
x.method()
```

is checked via the method set of `x`.

Whereas:

```text
T → Interface
```

additionally requires explicit conformance.

#### 6.1.6 Duplicate method names

Like Go, Anuy does not allow having simultaneously:

```anuy
func (value T) M() { ... }
func (value *T) M() { ... }
```

for one base named type.

A pointer receiver does not create a separate overload namespace.

There is no method overloading by receiver kind, parameters, or return type.

This preserves a direct correspondence to Go method sets.

#### 6.1.7 Receiver binding

The receiver of a method declaration is an ordinary parameter of the method body:

```anuy
func (file *File) Save() {
    file.path = "saved"
}
```

Rules:

- the receiver is spelled `T` or `*T`; a pointer receiver may carry `?` (`*T?` — nullable, RFC-002 §6.8.4). The form `T?` is not a receiver: the generated Go cannot define methods on the carrier type (RFC-009 §6.7);
- a named receiver (`file *File`, `file File`) is bound into the scope of the method body by the rules of RFC-003: it is a stable binding (RFC-003, RFC-002 §6.3.6), and its nullability class is determined by the spelling (`T` — non-null, `*T` — nil-tolerant platform semantics, `*T?` — nullable);
- the flow rules of RFC-002 apply to the receiver as to any binding: a nil check narrows a nullable receiver (§6.3.1–6.3.2), assignment to the receiver is allowed (it is local to the method) and invalidates narrowing per §6.3.4;
- an unnamed (`func (*T) M()`) and a blank (`func (_ *T) M()`) receiver are valid Go forms; no binding is introduced into the body;
- method sets and dispatch (§6.2) do not depend on the receiver name; duplicate names are checked per §6.1.6 regardless of the name.

### 6.2 Method Sets and Dispatch

#### 6.2.1 Value and pointer method sets

Anuy preserves the Go model.

For a named type `T`:

```text
MethodSet(T)
```

contains methods with receiver `T`.

```text
MethodSet(*T)
```

contains methods with receivers:

```text
T
*T
```

Example:

```anuy
func (file File) Name() string {
    ...
}

func (file *File) Read(buffer []byte) (int, error?) {
    ...
}
```

Then:

```text
File
    Name

*File
    Name
    Read
```

Therefore:

```anuy
impl Reader for File
```

is an error if `Read` has only a `*File` receiver.

Correct:

```anuy
impl Reader for *File
```

#### 6.2.2 Addressable method-call convenience

Go allows calling a pointer receiver method through an addressable value.

Anuy preserves this behavior.

```anuy
var file File = ...
file.Read(buffer)
```

MAY be valid as a method call if `file` is addressable.

Conceptually:

```text
file.Read(...)
→
(&file).Read(...)
```

But this **does not change the interface method set**.

Therefore the fact that:

```anuy
file.Read(buffer)
```

is valid does not mean that:

```anuy
impl Reader for File
```

is valid.

If `Read` has a pointer receiver:

```anuy
impl Reader for *File
```

remains the only correct declaration.

### 6.3 Multiple Interfaces and Embedding

#### 6.3.1 Multiple interfaces

One type MAY explicitly implement any number of interfaces:

```anuy
impl Reader for *File
impl Writer for *File
impl Closer for *File
```

If two interfaces require the same method signature, one method satisfies both.

If they require methods with the same name but incompatible signatures:

```anuy
interface A {
    Value() int
}

interface B {
    Value() string
}
```

one Go-compatible type cannot implement both.

The compiler diagnoses the incompatible `impl`.

#### 6.3.2 Interface embedding

Interfaces MAY explicitly extend other interfaces:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}

interface Writer {
    Write([]byte) (int, error?)
}

interface ReadWriter {
    Reader
    Writer
}
```

The effective method set of `ReadWriter` contains the methods of `Reader` and `Writer`.

Duplicate identical method requirements are merged.

Conflicting signatures are a compile error.

#### 6.3.3 Explicitness and embedded interfaces

If a type has:

```anuy
impl Reader for *File
impl Writer for *File
```

this does **not automatically mean**:

```anuy
impl ReadWriter for *File
```

Even if all methods are present.

To use `*File` directly as a `ReadWriter`, the programmer MUST declare the intent:

```anuy
impl ReadWriter for *File
```

This is the central rule of the RFC:

> Structural completeness does not create conformance by itself.

However, the feedback direction through explicit embedding is allowed.

If:

```anuy
impl ReadWriter for *File
```

then `*File` MAY be used where a `Reader` or a `Writer` is required, because `ReadWriter` itself explicitly declares these superinterfaces.

This is not accidental conformance: the relation is present directly in the declaration of `ReadWriter`.

#### 6.3.4 Interface-to-interface relations

In v1, a relation between native interfaces is created only by explicit embedding.

Not introduced:

```anuy
impl Reader for ReadWriter
```

For this, the following is used:

```anuy
interface ReadWriter {
    Reader
    ...
}
```

Two independently declared interfaces with identical method sets are not subtypes of each other.

### 6.4 Conversions and Nullability

#### 6.4.1 Conversions to interface values

If:

```anuy
impl Reader for *File
```

then normal implicit interface conversions are allowed:

```anuy
var file *File = ...
var reader Reader = file
```

as well as argument passing:

```anuy
func consume(reader Reader) {
    ...
}

consume(file)
```

and return conversion:

```anuy
func openReader() Reader {
    return openFile()
}
```

A separate explicit cast is not required: the explicit intent is already expressed by the `impl` declaration.

#### 6.4.2 Nullability

RFC-002 semantics apply to interfaces without exceptions.

```anuy
Reader
```

— a non-null interface value.

```anuy
Reader?
```

— a nullable interface value.

`nil` MUST NOT be assigned to `Reader`:

```anuy
var reader Reader = nil // error
```

but this is allowed:

```anuy
var reader Reader? = nil
```

Conformance is never declared for a nullable type:

```anuy
impl Reader for *File? // error
```

`*File?` denotes a nullable value, not a separate method-bearing nominal type.

The correct declaration:

```anuy
impl Reader for *File
```

After narrowing, the nullable value can be converted normally.

#### 6.4.3 Nil pointer receivers

A safe Anuy `*T` is a non-null type.

Therefore ordinary safe Anuy code cannot call a pointer receiver method on `nil`, even if the corresponding Go method could theoretically handle that.

```anuy
var file *File? = nil

file.Read(buffer) // error
```

After narrowing:

```anuy
if file != nil {
    file.Read(buffer)
}
```

the call is allowed.

Safe navigation also follows RFC-002.

Foreign Go typed-nil values belong to foreign boundary semantics and are governed by RFC-007/RFC-008.

#### 6.4.4 Interface-to-interface conversions

Design note, 2026-09-21 (slice design): §6.3.3 formulates the embedding feedback through concrete use (`*File` MAY be used as a `Reader`); a value-level rule for values whose static type is already an interface requires its own formulation. Industry precedent is subtyping through declared inheritance (C#, Java, Swift); Go gives a structural subset, which Anuy rejects for native interfaces (§10.1).

Let the source value have interface type `S` and the target be interface type `T`. The conversion `S → T` is valid if and only if:

1. `S` and `T` are the same interface type (identity);
2. `T` is reachable from `S` through a chain of explicit embedding (§6.3.2, transitively).

Every hop of the chain is declared in the source declaration, so transitivity preserves the central invariant of the RFC:

> **Nominal intent, structural verification, Go representation.**

All other directions are invalid: downcasts, sibling conversions, and conversion between two independently declared interfaces with identical method sets (§6.3.4). Dynamic type assertions and type switching on an interface value are defined by RFC-021.

Generated Go uses the ordinary Go interface conversion: after the Anuy checks, no runtime checks are generated (§6.8.4).

### 6.5 Coherence and Ownership

#### 6.5.1 Coherence

Anuy MUST have globally predictable conformance.

Therefore an ordinary `impl` is a package-level declaration.

Forbidden:

- local `impl`;
- block-scoped `impl`;
- import-scoped instances;
- "bring this impl into scope";
- competing implementations;
- specialization priority.

For one semantic pair:

```text
(interface, target type)
```

two overlapping `impl` declarations cannot exist.

#### 6.5.2 Type-owned impl rule

An ordinary `impl` MUST be declared in the package that owns the target nominal type.

For example:

```text
package files
```

defines:

```anuy
type File ...
```

Exactly this package MAY write:

```anuy
impl Reader for File
impl Reader for *File
impl io.Reader for *File
```

even if the interfaces are declared in other packages.

This is deliberately stricter than the classic orphan rule:

> **Conformance authority belongs to the concrete type owner.**

Reasons:

1. all interface commitments of a type are located next to the type;
2. a downstream package cannot change the meaning of a foreign type;
3. duplicate impl between the interface owner and the type owner is impossible;
4. the dependency direction remains clear;
5. interface evolution gives localized errors to the owner of the implementing type;
6. the generated Go methods already belong to the same package.

#### 6.5.3 Orphan implementations

Forbidden:

```anuy
// package app owns neither Reader nor somepkg.File

impl Reader for somepkg.File
```

Even if the method set matches.

A third-party type requires a local nominal wrapper/adaptor.

For example:

```anuy
type FileAdapter {
    file *somepkg.File
}

func (adapter *FileAdapter) Read(
    buffer []byte
) (int, error?) {
    return adapter.file.Read(buffer)
}

impl Reader for *FileAdapter
```

This makes ownership and the trust boundary explicit.

#### 6.5.4 Type aliases do not grant ownership

An alias:

```anuy
type MyFile = somepkg.File
```

does not allow:

```anuy
impl Reader for MyFile
```

if the current package does not own the original nominal type.

An alias does not create a new nominal type.

A distinct defined type does.

This prevents circumventing the orphan rule through an alias.

#### 6.5.5 Built-in and unnamed types

An ordinary native `impl` target MUST have an owned nominal type.

Therefore the following cannot be declared:

```anuy
impl Printable for int
impl Printable for []byte
impl Printable for map[string]int
```

from an arbitrary package.

Where necessary, a local wrapper/newtype is used.

This also guarantees that conformance does not appear unexpectedly for pervasive built-in types.

Design note, 2026-09-21 (friction profile): the practical cost of the rule is concentrated in one case — "a foreign/stranger type → our interface" (adapter, §6.5.3). The other two frequent cases are free: our type → a foreign interface (impl in the package of the type owner, including foreign interfaces — §6.5.2, §6.7.1) and Go type → Go interface (§6.7.2, the structural rules of Go). The adapter is mechanically cheap for narrow interfaces; candidates for making it cheaper are embedding promotion (if method promotion is modeled), the RFC-008 `foreign impl` sugar for ABI-identical cases, and centralized ecosystem adapter packages. The alternative "the interface owner may impl" was rejected: it reopens duplicate-impl coherence between the interface owner and the type owner (§6.5.2.3) and loses the property that "all commitments live next to the type".

### 6.6 Generics

#### 6.6.1 Generic interfaces

Interfaces MAY be generic:

```anuy
interface Source[T any] {
    Next() (T, bool)
}
```

Instantiation creates a concrete interface type:

```text
Source[int]
Source[string]
```

These are distinct interface types.

#### 6.6.2 Generic impl

A generic owned type MAY declare a generic conformance:

```anuy
type Stream[T any] {
    ...
}

func (stream *Stream[T]) Next() (T, bool) {
    ...
}

impl[T any] Source[T] for *Stream[T]
```

Constraints MAY use the ordinary Anuy generic constraint syntax:

```anuy
impl[T Serializable] Encoder[T] for *Codec[T]
```

RFC-004 does not introduce a separate `where` language.

#### 6.6.3 Generic impl coherence

There is no specialization.

Two `impl` declarations MUST NOT exist if there is a valid substitution of type parameters under which they describe the same pair:

```text
(interface instantiation, concrete target type)
```

For example, overlapping declarations are an error.

Compiler MUST reject overlap.

If disjointness cannot be reliably proven, the v1 compiler MAY conservatively reject the declarations instead of choosing the "more specific" `impl`.

There is no:

- specialization;
- priority;
- fallback impl;
- negative impl.

Every type parameter of a generic `impl` MUST participate in the target type or the interface type, so that the declaration contains no unconstrained existential parameters.

#### 6.6.4 Interfaces as generic constraints

A native interface MAY be used as a generic constraint.

```anuy
func copy[T Reader](value T) {
    ...
}
```

For a native Anuy type, satisfying such a constraint requires the same explicit conformance as an ordinary interface conversion.

The presence of suitable methods without `impl Reader for T` is not enough.

Thus generic constraints do not create an alternate structural-conformance path.

### 6.7 Go Interoperability

#### 6.7.1 Go interfaces

An imported Go interface retains its own Go identity and method set.

For example:

```go
io.Reader
```

is available in Anuy as a foreign Go interface type.

For a native Anuy type, the transition to a Go interface MUST also express programmer intent:

```anuy
impl io.Reader for *File
```

The compiler verifies that the generated Go method set of `*File` satisfies `io.Reader`.

After that:

```anuy
var reader io.Reader = file
```

is allowed.

#### 6.7.2 Go type → Go interface

If both the concrete type and the interface are foreign Go entities:

```text
Go type → Go interface
```

Anuy does not invent an additional conformance model.

The standard Go structural rules apply.

This is foreign Go semantics.

For example, an imported Go `*os.File` can be used as an imported Go `io.Reader` if Go considers it an implementer.

An additional Anuy `impl` is not needed for this and, according to the ownership rule, is usually impossible.

#### 6.7.3 Go type → native Anuy interface

An ordinary declaration:

```anuy
impl Reader for *os.File
```

from an Anuy package is forbidden, because the package does not own `os.File`.

The canonical safe mechanism in v1 is a local adapter:

```anuy
type OSFileReader {
    file *os.File
}

func (reader *OSFileReader) Read(
    buffer []byte
) (int, error?) {
    return reader.file.Read(buffer)
}

impl Reader for *OSFileReader
```

Such a design makes the foreign/native boundary explicit.

RFC-008 MAY later define a zero-cost `foreign impl` sugar for ABI-identical cases, but the ordinary `impl` MUST NOT become an orphan escape hatch.

#### 6.7.4 Interoperability matrix

Normative model:

```text
Anuy type → Anuy interface
    explicit impl
    target type owner declares it

Anuy type → Go interface
    explicit impl
    target type owner declares it

Go type → Go interface
    standard Go structural semantics

Go type → Anuy interface
    local adapter / foreign mechanism
    no ordinary orphan impl
```

This is the base compatibility matrix of RFC-004.

#### 6.7.5 Mixed `.go` + `.anuy` packages

Methods generated from `.anuy` files and methods from `.go` files MAY participate in the Go method set of one package according to the interop rules.

However, native Anuy conformance still requires an explicit `impl` declaration.

If a method required by a native interface is located in `.go`, the compiler MAY use it as a structural witness only after the foreign contract rules of RFC-007/RFC-008 are applied.

The mere fact that a method is present in a mixed package MUST NOT automatically bypass the trust boundary.

#### 6.7.6 Why explicit `impl` is required even for Go interfaces

The Go compiler could structurally accept `*File` as `io.Reader` without a declaration.

Anuy deliberately requires:

```anuy
impl io.Reader for *File
```

because the compatibility interface is part of the programmer intent.

This makes it possible to:

- find all externally promised interfaces next to the type;
- get a localized error when a method signature changes;
- distinguish accidental coincidence of methods from an intentional contract.

The generated Go remains standard Go and structurally satisfies the interface.

### 6.8 Lowering and ABI

#### 6.8.1 Native interface lowering

A native Anuy interface MUST lower to an ordinary Go interface.

Conceptually:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}
```

becomes a Go interface with equivalent lowered method signatures.

The exact representation of nullable values and other Anuy-specific types is defined by RFC-008/RFC-009.

But RFC-004 requires:

> interface dispatch itself MUST use ordinary Go interface dispatch.

There MUST NOT be a separate Anuy runtime dispatch.

#### 6.8.2 `impl` lowering

The declaration:

```anuy
impl Reader for *File
```

usually generates no executable Go code.

Compiler:

1. resolves `Reader`;
2. resolves `*File`;
3. computes effective method set;
4. validates all required methods;
5. records conformance in semantic IR/export metadata;
6. allows corresponding Anuy conversions.

After that, the `impl` can be erased for Go code generation.

Not needed:

```text
witness tables
marker fields
registries
vtable objects
runtime lookup
```

#### 6.8.3 No synthetic marker methods

RFC-004 v1 deliberately does not add a hidden Go method such as:

```go
AnuyImplementsReader()
```

for each `impl`.

Such an approach is rejected because it:

- pollutes Go method sets;
- degrades direct Go interoperability;
- becomes complicated for generic interface instantiations;
- creates ABI-visible compiler artifacts;
- prevents one type from implementing different instantiations of generic interfaces;
- effectively turns source-level explicitness into a runtime nominal mechanism.

Explicit `impl` is a guarantee of the Anuy compiler, not a new runtime identity system.

#### 6.8.4 ABI

RFC-004 requires:

> Explicit conformance MUST NOT require a new calling convention.

The interface call:

```anuy
reader.Read(buffer)
```

MUST lower to an ordinary Go interface call.

The concrete method call:

```anuy
file.Read(buffer)
```

MUST lower to an ordinary Go method call.

`impl` adds no hidden parameters.

This preserves:

- the Go calling convention;
- Go interface dispatch;
- the Go reflection representation, as far as the lowered type representation allows;
- Delve compatibility;
- ordinary stack frames;
- standard Go method sets.

#### 6.8.5 Generated Go

Conceptually:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}

func (file *File) Read(
    data []byte
) (int, error?) {
    ...
}

impl Reader for *File
```

lowering MUST NOT add a separate `impl` construct.

Conceptually generated Go:

```go
type Reader interface {
    Read([]byte) (int, error)
}

func (file *File) Read(data []byte) (int, error) {
    ...
}
```

provided that the concrete Anuy representations of these types are indeed ABI-equivalent.

If `error?` or other types require a bridge representation, RFC-008/RFC-009 define the exact generated signature.

But the interface machinery remains Go-native.

### 6.9 Foreign Boundary, Evolution and Metadata

#### 6.9.1 Consequence for Go boundaries

Since the generated native interface remains an ordinary Go interface, external Go code can theoretically construct an interface value structurally without an Anuy `impl`.

This is not considered a violation of RFC-004.

Such a value arrived through the foreign boundary.

The rule:

> **Explicit conformance is an Anuy source-language invariant, not a claim that arbitrary Go code cannot construct structurally compatible values.**

Trust rules for returning such a value back into safe Anuy belong to RFC-007/RFC-008.

This matches the general native vs foreign model.

#### 6.9.2 Empty interfaces / marker interfaces

A native interface MAY be empty:

```anuy
interface Serializable {}
```

In Anuy, explicitness still applies:

```anuy
impl Serializable for User
```

is required.

That is, an empty native interface does not mean "any Anuy type".

In generated Go, such an interface can have an empty structural representation.

Consequently, Go code is able to pass other values through the foreign boundary.

This is the same intentional foreign-boundary rule, not a reason to introduce a synthetic marker method.

#### 6.9.3 Interface evolution

Let:

```anuy
interface Reader {
    Read([]byte) (int, error?)
}
```

and:

```anuy
impl Reader for *File
```

If the interface gains a new required method:

```anuy
interface Reader {
    Read([]byte) (int, error?)
    Close() error?
}
```

the compiler reports an error directly at:

```anuy
impl Reader for *File
```

indicating the missing method.

This is one of the key reasons for explicit impl.

Structural conformance is not accidentally searched across the whole codebase.

#### 6.9.4 Package metadata

`impl` is not a Go ABI object, but it is part of the Anuy semantic API.

A downstream Anuy compiler MUST know the exported conformance facts of an imported package.

Therefore the compiler export metadata MUST contain the `impl` declarations needed for downstream type checking.

Removing a public conformance:

```anuy
impl Reader for *File
```

can be a source-breaking change for Anuy consumers even when the generated Go method set has not changed.

Go-only consumers of the generated package do not see this semantic distinction.

The export metadata format is defined by the tooling/lowering RFC.

#### 6.9.5 Final semantic model

RFC-004 fixes four distinct notions.

**Method ownership** — the method belongs to the concrete type:

```anuy
func (file *File) Read(...) ...
```

**Structural capability** — the compiler computes the Go-compatible method set.

**Explicit intent** — the programmer declares:

```anuy
impl Reader for *File
```

**Runtime representation** — a `Reader` value uses the ordinary Go interface mechanism.

In short:

```text
methods
    ↓
structural method set
    ↓
explicit impl declaration
    ↓
checked conformance
    ↓
ordinary Go interface representation
```

## 7. Interaction with Other RFCs

### 7.1 RFC-002

Nullability applies to interfaces without exceptions (Section 6.4.2): a native interface is non-null, the nullable form is `I?`; `T?` is not an impl target. Nullable concrete→interface boxing and the typed nil policy are a joint decision with RFC-008 (Section 12).

### 7.2 RFC-003

Method receivers participate in lexical scope by the rules of RFC-003; methods are not declared inside `impl`.

### 7.3 RFC-007 / RFC-008

Foreign boundary: Go typed-nil, structural values through the foreign boundary, mixed `.go`+`.anuy` packages, and trust rules — RFC-007/RFC-008 (Sections 6.7.5, 6.9.1); the `foreign impl` sugar is a possible future extension of RFC-008 (Section 6.7.3).

### 7.4 RFC-009 / RFC-010

The representation of nullable values in interface signatures, the exact generated signatures, and the export metadata format — RFC-009/RFC-010 (Sections 6.8, 6.9.4).

### 7.5 RFC-011

SourceMap identity of `impl`, the LSP surface (go to implementation, interface hierarchy) — RFC-011 (Sections 8.2–8.3).

## 8. Diagnostics and Tooling

### 8.1 Diagnostics Catalog

Identifiers D-1…D-4 are local references within the RFC; message forms are illustrative; stable `ANUY####` codes follow RFC-011 policy. Diagnostics SHOULD be centered around the intent declaration.

#### 8.1.1 D-1 — Missing Method

```text
error: *File does not implement Reader

    impl Reader for *File
         ^^^^^^

missing method:
    Close() error?
```

#### 8.1.2 D-2 — Receiver Mismatch

```text
error: File does not implement Reader

required:
    Read([]byte) (int, error?)

available:
    func (*File).Read([]byte) (int, error?)

hint:
    only *File has this method in its method set;
    did you mean:

    impl Reader for *File
```

#### 8.1.3 D-3 — Orphan Violation

```text
error: cannot declare an impl for foreign type os.File

    impl Reader for *os.File
                    ^^^^^^^^

ordinary impl declarations belong to the package
that owns the target nominal type

hint:
    define a local adapter type
```

#### 8.1.4 D-4 — Missing Explicit Declaration

```text
error: File cannot be used as Reader

File structurally provides all required methods,
but no explicit conformance is declared

hint:
    add in File's package:

    impl Reader for File
```

#### 8.1.5 D-5 — Undefined Interface Member

Design note, 2026-09-21 (slice design): for a receiver with a known interface type, the available method set is known exactly (§6.1.1), so a call to a missing method is a definite error, not a tolerance zone. Contrast: unknown concrete receivers without a declared type remain in the tolerance zone (RFC-002 §8.2). Precedent — Go, C#, Java, where member lookup on a known interface type is fully checked.

```text
error: Reader has no method Reaad

    reader.Reaad(buffer)
           ^^^^^

available interface methods:
    Read([]byte) (int, error?)
```

### 8.2 SourceMap

User-written methods map normally to their Anuy source spans.

`impl` declarations have no generated executable span in the common case, but MUST retain semantic SourceMap/diagnostic identity.

Semantic IR SHOULD retain at least:

```text
ImplID
InterfaceType
TargetType
DeclarationSpan
MethodWitnesses
```

where `MethodWitnesses` associates each required interface method with the concrete method declaration that proved conformance.

This is required for:

- diagnostics;
- IDE navigation;
- "go to implementation";
- interface hierarchy;
- rename analysis;
- refactoring;
- compiler explanations.

If future interop lowering creates synthetic wrappers, they MUST use `SourceKind = InteropWrapper` and refer back to the corresponding Anuy declaration.

### 8.3 LSP Semantics

For:

```anuy
impl Reader for *File
```

the IDE SHOULD support:

- go to interface;
- go to concrete type;
- list interface implementations;
- list interfaces implemented by type;
- show missing methods;
- navigate from required method to implementing method.

On navigation:

```anuy
Reader.Read
```

→ implementation

The IDE MAY use the recorded `MethodWitnesses`.

This does not require analyzing the generated Go as the canonical semantic source.

## 9. Rationale

### 9.1 Why explicit `impl` is required even for Go interfaces

The Go compiler could structurally accept `*File` as `io.Reader` without a declaration.

Anuy deliberately requires:

```anuy
impl io.Reader for *File
```

because the compatibility interface is part of the programmer intent.

This makes it possible to:

- find all externally promised interfaces next to the type;
- get a localized error when a method signature changes;
- distinguish accidental coincidence of methods from an intentional contract.

The generated Go remains standard Go and structurally satisfies the interface.

## 10. Rejected Alternatives

### 10.1 Implicit structural conformance

Rejected:

```text
if method set matches interface,
the type automatically implements it
```

This is the ordinary Go semantics.

Anuy rejects it for native conformance, because accidental matching is impossible to distinguish from an intentional API commitment.

This is especially important for:

- large method sets;
- generic libraries;
- refactoring;
- adding interfaces after concrete types already exist;
- interface evolution.

### 10.2 Methods inside interface impl

Rejected:

```anuy
impl Reader for File {
    func Read(...) {
        ...
    }
}
```

Problems:

- the method starts to belong simultaneously to the type and to the interface implementation;
- several interfaces can require the same method;
- identical method names create ambiguity;
- generated Go still requires a single method on the concrete type;
- specialization and conflict resolution become a separate language subsystem.

Anuy does not require this.

### 10.3 Inherent `impl T`

Rejected for v1:

```anuy
impl File {
    func Read(...) {
        ...
    }
}
```

Such syntax exists in other languages, but it does not give Anuy enough benefit to replace the clear Go receiver syntax.

Go-style methods:

```anuy
func (file *File) Read(...) {
    ...
}
```

preserve source familiarity and lowering transparency.

### 10.4 Classic orphan rule

A more permissive rule:

```text
impl allowed when current package owns either
the interface or the concrete type
```

is not adopted.

It allows the interface owner to retroactively declare conformance for types from other packages.

This creates potential duplicate declarations and distributes the commitments of one concrete type across several packages.

RFC-004 chooses the stronger rule:

> ordinary impl belongs to the target type owner.

Foreign types use adapters.

### 10.5 Interface witness tables

A Rust-like compile-time/runtime witness-table architecture is not required.

Go already provides the necessary runtime interface mechanism.

Adding a separate Anuy dispatch layer would violate:

> Go semantics with stronger invariants.

In Anuy, the stronger invariant lives in compiler checking, not in a new runtime.

## 12. Open Questions

- **OQ-1 Nullable boxing and typed nil.** The exact policy for nullable concrete → non-null interface (prohibited / Go typed-nil semantics preserved / through a nullable interface) and typed-nil interface behavior is a joint decision of RFC-004 and RFC-008 (delegated to RFC-002, Sections 6.9.17 and 12.6–7).
- **OQ-2 Export metadata format.** The export metadata format for `impl` facts (Section 6.9.4) is defined by RFC-009/RFC-010/RFC-011.

## 13. Normative Summary

The following rules are adopted for v1:

1. Native Anuy interfaces have nominal identity.
2. Interface requirements consist of methods and embedded interfaces.
3. Ordinary methods are declared with the Go-style receiver syntax.
4. Methods are not declared inside `impl`.
5. Native interface conformance is always explicit.
6. The conformance declaration syntax:

   ```anuy
   impl Interface for Type
   ```

7. `impl` has no body.
8. Direct method calls do not require an interface `impl`.
9. Value/pointer method sets follow Go.
10. Addressability convenience for method calls does not change the interface method set.
11. One type MAY explicitly implement several interfaces.
12. Structural compatibility by itself does not create conformance.
13. Interface embedding creates an explicit interface hierarchy.
14. `impl Child for T` allows using `T` as the embedded parents.
15. An ordinary `impl` is declared only in the owner package of the target nominal type.
16. Orphan impl is forbidden.
17. A type alias does not create ownership.
18. A foreign Go type → Anuy interface uses an adapter/foreign mechanism.
19. A native Anuy type → Go interface requires explicit `impl`.
20. Go type → Go interface uses Go structural semantics.
21. Generic interfaces are allowed.
22. Generic impl is allowed.
23. Overlapping generic impl is forbidden.
24. There is no specialization.
25. `T?` is not a separate impl target.
26. A native interface `I` is non-null; the nullable interface is `I?`.
27. Native interfaces lower to ordinary Go interfaces.
28. `impl` creates no runtime dispatch objects.
29. Synthetic marker methods are not used.
30. `impl` is part of the Anuy semantic package API and export metadata.

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (design filter);
- RFC-001 — Values, Initialization and Trust (method receivers as bindings);
- RFC-002 — Nullability and Nil Safety (nullable interfaces, impl targets);
- RFC-007 — Unsafe and Foreign Contracts (foreign boundary, trust rules);
- RFC-008 — Go Interoperability (typed nil, mixed packages, foreign impl sugar);
- RFC-009 — Lowering and Generated Go Contract (representation in interface signatures);
- RFC-011 — Diagnostics, Debugging and Tooling (SourceMap, LSP, codes).

### 14.2 Informative

- Methods proposal (2026-09-17) — implementation of method declarations in the experimental grammar.
