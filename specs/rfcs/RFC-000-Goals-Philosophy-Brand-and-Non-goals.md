# RFC-000 — Goals, Philosophy, Brand and Non-goals

**Status:** Accepted
**RFC:** 000
**Title:** Goals, Philosophy, Brand and Non-goals
**Language:** Anuy
**Area:** Foundational design filter / Philosophy / Brand / Governance
**Version:** 3
**Date:** 2026-09-19
**Requires:** —
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-000 was accepted by the project owner as the normative foundational design filter of Anuy.

The dependency review confirmed that RFC-001—RFC-011 and the related working drafts do not redefine its normative principles: stronger correctness guarantees, explicit intent, Go interoperability, and conceptual simplicity.

Questions from [Section 12](#12-open-questions) are explicitly delegated to other RFCs or to a future Language Core/reference manual and do not block the acceptance of RFC-000.

2026-09-19: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 2 → 3.

---

## 1. Abstract

Anuy is a statically typed programming language for the Go ecosystem. Anuy preserves the fundamental Go model (runtime, toolchain, package model, modules, standard library, goroutines, channels, garbage collection, interfaces, error values, simple imperative control flow) and adds stronger compile-time guarantees where Go relies on implicit behavior, convention, or runtime failure. Anuy is not a new backend/runtime ecosystem and must not become a self-contained technological island: Anuy source code is lowered to an ordinary Go representation and built with the standard Go toolchain. RFC-000 is accepted as the normative foundational design filter of the entire RFC set.

## 2. Solutions

> **Make important programmer intent part of the verifiable specification of the program, without destroying the simplicity and the engineering model of Go.**

> **Explicit where correctness depends on intent. Simple where intent is unambiguous.**

> **Safe Anuy source cannot by itself create a state forbidden by the Anuy type system.**

> **Anuy MUST prioritize stronger correctness guarantees, explicit intent, Go interoperability, and conceptual simplicity over syntactic novelty and maximal language expressiveness.**

> **Anuy SHOULD own the semantics of the program, but SHOULD reuse Go as much as possible for the runtime, the compilation backend, and ecosystem infrastructure.**

> **Anuy is Go-compatible by design.**

> **Anuy makes important programmer intent explicit and statically checkable.**

> **Anuy owns language semantics; Go remains the execution ecosystem.**

## 3. Motivation

Go is successful largely thanks to a limited set of language features, a simple runtime model, a fast compiler/toolchain, and a predictable programming style.

However, the same philosophy leads to a class of problems where programmer intent is not expressed strictly enough.

Examples:

- a pointer-like value can be `nil`, although the API logically requires a value;
- a variable automatically receives a zero value, even if for the type such a state is not a valid or meaningful default;
- a type structurally implements an interface without an explicit declaration of intent;
- a returned `error` can be ignored;
- closed domain values have to be modeled through conventions;
- short declaration `:=` merges declaration and assignment;
- part of the contracts of existing Go APIs exists only in documentation;
- some invalid states are detected only by a runtime panic, or far from where they were created.

Anuy is intended to eliminate part of these problems at the compile-time level.

At the same time, the goal is not to fix every subjectively inconvenient feature of Go.

Anuy must remain recognizably close to Go in mental model.

## 4. Goals

### 4.1 Stronger Compile-Time Guarantees

Anuy MUST strive to move certain classes of errors from runtime or code review to compile time.

The main target categories:

- invalid null usage;
- invalid implicit initialization;
- accidental interface conformance;
- ignored errors;
- non-exhaustive handling closed domains;
- violations of explicitly defined type invariants.

### 4.2 Explicit Programmer Intent

Anuy SHOULD require explicit syntax or declaration where programmer intent substantially affects correctness.

Examples of intended solutions:

- nullable types are marked explicitly;
- interface implementation is marked explicitly;
- ignoring an `error` is marked explicitly;
- an unsafe assumption is marked explicitly;
- declaration without initializer explicitly leaves the binding uninitialized until ordinary assignment.

### 4.3 Preserve Go Simplicity

Anuy MUST treat simplicity as an independent design constraint.

A new feature SHOULD:

- have a small semantic surface;
- be locally understandable;
- have a limited number of special cases;
- not require knowledge of a complex runtime model;
- interact naturally with existing Go.

If two design alternatives provide comparable guarantees, the simpler one SHOULD be chosen.

### 4.4 Excellent Go Interoperability

Interop is a primary goal, not a secondary feature.

Anuy MUST support the scenarios:

```text
Go → Anuy
Anuy → Go
```

Anuy SHOULD support mixed packages:

```text
foo.go
bar.anuy
```

and mixed modules.

An existing Go project SHOULD be able to adopt Anuy gradually without a full rewrite.

### 4.5 Progressive Adoption

Anuy MUST be suitable for gradual migration.

An acceptable lifecycle:

```text
100% Go
 ↓
Go + a few .anuy
 ↓
mixed codebase
 ↓
Anuy-dominant codebase
```

A project MAY remain mixed-language forever.

Anuy MUST NOT require full migration of a package/module just for the sake of using the language.

### 4.6 Pure-Go Consumption

A published library written in Anuy SHOULD be able to provide a representation suitable for use by an ordinary Go project without the Anuy compiler installed.

The preferred model:

```text
Anuy source
 ↓
anuy emit-go
 ↓
ordinary Go module/package representation
```

Exact publishing rules are defined by a separate RFC.

### 4.7 Predictable Lowering

A programmer SHOULD be able to reasonably understand the runtime cost of Anuy constructs.

Anuy SHOULD avoid constructs whose simple source representation hides a disproportionately complex runtime behavior.

For example, error propagation:

```text
try operation()
```

may lower to several Go statements, but SHOULD NOT hide exceptions, scheduler transitions, or heap allocation without an explicit need.

### 4.8 Good Observability

An Anuy program MUST remain understandable in:

- compiler diagnostics;
- the debugger;
- stack traces;
- coverage reports;
- IDE navigation.

Generated Go MUST NOT become the user-facing source of truth.

Source-level identity belongs to `.anuy`.

## 5. Non-goals

### 5.1 Replacing the Go Runtime

Anuy does not aim to create:

- its own garbage collector;
- its own scheduler;
- its own goroutine equivalent;
- its own memory allocator;
- an alternative runtime ABI.

### 5.2 Replacing the Go Compiler Backend

Anuy does not aim to write its own machine-code backend.

The target model:

```text
Anuy frontend
 ↓
Go representation
 ↓
Go compiler
```

This decision MAY be revised only by a separate foundational RFC.

### 5.3 Exceptions

Anuy does not aim at the introduction of exception-based error handling.

Go-style:

```text
(T, error)
```

is the basic interoperability model.

Anuy MAY improve the syntax and the compile-time handling of errors, but SHOULD preserve this ABI where an API interacts with Go.

### 5.4 Mandatory Result Type

Anuy does not aim to replace standard Go errors with a mandatory:

```text
Result<T, E>
```

General-purpose sum types MAY exist separately, but must not automatically replace the Go error model.

### 5.5 Rust-Style Ownership

Anuy does not aim at a full-fledged:

- borrow checker;
- ownership system;
- lifetime calculus;
- compile-time memory management model of Rust.

This would substantially change the mental model and the compatibility with Go.

### 5.6 Compile-Time Race Freedom

Anuy does not promise the static absence of data races.

It MAY add individual safety features around concurrency if they fit the Go model well, but this is not a core v1 goal.

### 5.7 Full Effect System

Anuy does not aim at a complex effect system for tracking:

- IO;
- allocation;
- panic;
- blocking;
- concurrency;
- mutation.

Error propagation by itself must not automatically lead to a general effect system.

### 5.8 Arbitrary Metaprogramming

Anuy does not aim at:

- Rust-style general macros;
- AST macros;
- arbitrary compile-time code execution;
- compiler plugins that change the semantics of the language.

Code generation MAY exist as a tooling mechanism, but not necessarily as part of the core language.

### 5.9 Object-Oriented Class Hierarchy

Anuy does not aim at:

- class inheritance;
- virtual class hierarchy;
- protected members;
- traditional OO object model.

Composition and the Go interface model remain the foundation.

### 5.10 Syntax Innovation for Its Own Sake

Anuy must not change Go syntax just because another syntax is:

- shorter;
- trendier;
- visually different;
- reminiscent of Kotlin/Rust/Swift.

Syntax changes must have a semantic justification or significantly remove ambiguity.

### 5.11 Explicitly Deferred Areas

The following areas are deliberately deferred:

- general sum types;
- advanced pattern matching;
- concurrency safety extensions;
- foreign contract declaration format;
- package-wide safe Go analysis;
- self-hosting;
- macro system;
- general compile-time execution;
- advanced effect tracking.

Their absence is not a blocker for Anuy v1.

### 5.12 Self-Hosting

The Anuy compiler is not required to be written in Anuy.

Self-hosting is not a language goal.

The initial compiler SHOULD be written in Go, because this:

- reduces bootstrap complexity;
- eases Go tooling integration;
- simplifies distribution;
- allows the language to be stabilized independently of the compiler implementation language.

Self-hosting MAY be considered after the language is stabilized.

## 6. Specification

### 6.1 Core Design Principle

The main principle of Anuy:

> **Explicit where correctness depends on intent. Simple where intent is unambiguous.**

> **Explicitness where a hidden decision affects correctness. Simplicity where the intent is unambiguous.**

This means, for example:

```text
var x = expression
```

may use type inference, because the intent is unambiguous.

But an implicit nullable type is undesirable, because the nullable/non-null distinction directly affects correctness.

Similarly:

```text
discard operation()
```

may be required for deliberately ignoring an error, because the intent cannot be safely inferred from the absence of usage of the result.

### 6.2 Product Identity, Naming and Brand

#### 6.2.1 Product Identity

Anuy is not:

- a new syntax dialect for the sake of a more modern appearance;
- a macro preprocessor;
- a library on top of Go;
- fork Go compiler;
- an alternative runtime;
- an attempt to create "Rust with Go syntax";
- an attempt to create "Kotlin with the Go standard library";
- an unofficial Go 2.

Anuy is a separate language, but a language whose ecosystem identity is closely tied to Go.

The preferred short description:

> **Anuy is a safer, more explicit language for the Go ecosystem.**

An acceptable technical description:

> **Anuy is a statically typed frontend that lowers to Go while enforcing stronger source-level invariants.**

#### 6.2.2 Name

The name of the language:

```text
Anuy
```

The standard extension of source files:

```text
.anuy
```

The main CLI:

```text
anuy
```

Examples:

```text
main.anuy
server.anuy
config.anuy
```

```text
anuy build
anuy run
anuy test
anuy check
anuy fmt
```

#### 6.2.3 Origin of the Name

The name comes from the Anuy River in Altai.

The geographic origin is part of the brand identity of the language, but not part of its technical semantics.

The river also creates a natural architectural metaphor:

```text
Anuy
 ↓
lowering
 ↓
Go
```

This idea can be expressed with the phrase:

> **Anuy flows to Go.**

It MAY be used in the branding, documentation, and visual language of the project.

It MUST NOT influence design terminology in a way that degrades technical clarity.

#### 6.2.4 Brand Philosophy

Anuy should be perceived as an independent engineering project.

The brand SHOULD convey:

- calmness;
- technical rigor;
- a connection to the Go ecosystem;
- simplicity;
- reliability.

The brand SHOULD NOT be built around the idea:

> Go is bad, Anuy fixes it.

The preferred framing:

> Go is a strong foundation; Anuy adds stricter contracts where they are useful.

#### 6.2.5 Visual Identity

The visual identity MAY use:

- the river;
- the flow;
- Altai;
- the landscape;
- an engineering/technical mascot;
- visual kinship with the Go ecosystem.

Anuy SHOULD avoid directly copying Go branding or creating the impression of an official Go variant.

#### 6.2.6 Relationship to Go Community

The project SHOULD strive to be technically respectful of Go's decisions.

When criticizing a specific Go feature, the project SHOULD distinguish:

- a deliberate design trade-off;
- a historical limitation;
- an incompatibility with the goals of Anuy.

Anuy must not build its identity exclusively on criticizing upstream Go.

### 6.3 Relationship to Go

#### 6.3.1 Go as the Runtime and Interoperability Platform

Anuy MUST treat the Go ecosystem as its primary runtime and interoperability platform.

Anuy MUST strive to reuse:

- the Go runtime;
- the Go compiler;
- the Go linker;
- Go modules;
- the Go standard library;
- existing Go libraries;
- existing mature Go tooling components where technically possible.

Anuy SHOULD NOT create an alternative equivalent of existing Go infrastructure without a substantial reason.

#### 6.3.2 Go Is Not Merely an Intermediate Language

Generated Go is not an incidental implementation detail like a general-purpose IR.

Go is part of the compatibility contract of Anuy.

Every substantial Anuy feature MUST have a defined representation or lowering strategy in Go.

A language feature SHOULD have a high adoption threshold if its implementation requires:

- a complex generated runtime;
- opaque wrappers around most values;
- substantial incompatibility with ordinary Go APIs;
- significant runtime overhead;
- the impossibility of reasonably using the generated API from Go.

#### 6.3.3 Standard Library Philosophy

Anuy does not aim to create an independent standard library duplicating the Go stdlib.

Preferably:

```text
fmt
os
net/http
context
sync
...
```

are used directly.

Anuy MAY provide a small language support/runtime package if individual features cannot be reasonably lowered without helpers.

Such a runtime MUST remain minimal.

#### 6.3.4 Ecosystem Principle

Anuy should strive for the following state:

```text
Go developer
    ↓
learns several stricter Anuy rules
    ↓
can productively use Anuy
```

and not:

```text
Go developer
    ↓
must relearn runtime,
package system,
dependency system,
error model,
concurrency model
```

Anuy language additions SHOULD concentrate precisely in the areas where stronger guarantees justify the new semantics.

### 6.4 Safety and Foreign Boundary

#### 6.4.1 Native and Foreign Worlds

Anuy MUST acknowledge that the Go type system is weaker than some intended Anuy invariants.

Therefore, the semantic model distinguishes the origin of a value.

**Native Value** — a value created by safe Anuy code or verified by an Anuy validation mechanism.

**Foreign Value** — a value obtained from Go or another boundary where Anuy guarantees cannot be considered proven.

This distinction MAY not always be directly visible in the syntax, but MUST be accounted for by the language semantics.

#### 6.4.2 Safety Boundary

The core safety guarantee of Anuy:

> **Safe Anuy source cannot by itself create a state forbidden by the Anuy type system.**

This does not mean:

> Go code in the same process/package is physically incapable of violating an Anuy invariant.

Go remains the interoperability escape boundary.

If Go can create an invalid representation, Anuy MUST define one or more of:

- conservative foreign type;
- validation;
- unsafe assumption;
- rejection of interop scenario.

#### 6.4.3 Unsafe Philosophy

Anuy MAY have `unsafe`.

`unsafe` means:

> The programmer asserts an invariant that the compiler is unable to prove.

`unsafe` MUST NOT mean:

> Type checking is disabled.

Inside an unsafe context, the ordinary language rules SHOULD continue to apply.

Unsafe operations SHOULD be a closed and explicitly specified set.

### 6.5 Language Directions

#### 6.5.1 Initialization Philosophy

Anuy rejects implicit source initialization. The declaration:

```text
var x T
```

creates an uninitialized binding, not a value of `T` and not a call of a special contract. The binding can be read only after the compiler has proven initialization on every reachable path.

The Go zero representation MAY be used by the backend as a storage detail, but it is not an Anuy source value until explicit initialization.

The detailed semantics are defined by RFC-001.

#### 6.5.2 Nullability Philosophy

Anuy SHOULD treat the absence of a value as part of the type system.

Non-null SHOULD be the default for types for which the absence of a value is a separate semantic state.

The nullable state must be explicit.

At the same time, the nil-capable Go categories:

- pointer;
- map;
- slice;
- channel;
- func;
- interface;

MUST be analyzed separately, since their Go semantics differ.

#### 6.5.3 Interface Philosophy

Anuy preserves the value of Go interfaces, but considers the declaration of intent important for native type relationships.

Explicit conformance is intended:

```text
impl Interface for Type
```

This must provide:

- intentional conformance;
- local diagnostics;
- no accidental implementation for native types.

At the same time, Anuy MUST preserve interoperability with structural Go interfaces.

#### 6.5.4 Error Philosophy

`error` remains an ordinary Go-compatible error value.

Anuy SHOULD strengthen error handling predominantly through compile-time restrictions.

The main direction:

> An error result must not be lost implicitly.

Normative forms:

```text
try operation()
```

```text
discard operation()
```

or explicit assignment/handling.

Anuy SHOULD avoid hidden exception machinery.

#### 6.5.5 Variable Philosophy

Anuy does not carry over the Go short declaration:

```text
:=
```

The main reason is the separation of declaration and assignment.

Intended:

```text
var x = expression
x = expression
```

Each form has one unambiguous meaning.

#### 6.5.6 Closed Values

Anuy MAY provide typed enums and closed variant sets if they have:

- a simple semantic model;
- a reasonable Go representation;
- predictable invalid-state behavior;
- exhaustive handling.

Exhaustiveness is a compile-time guarantee and matches the core philosophy of Anuy.

### 6.6 Generated Go and Source Identity

#### 6.6.1 Generated Go

Generated Go is:

- the compilation representation;
- the Go interoperability representation;
- a bridge to Go tooling.

Generated Go is not:

- the canonical source;
- a stable representation of all internal details;
- a mandated target for user editing.

#### 6.6.2 Public vs Internal Generated Contract

Two levels must be distinguished.

**Public Generated Contract** — declarations/representations used by pure-Go consumers. Such elements MAY have compatibility guarantees.

**Internal Lowering Representation** — compiler-generated:

- temporaries;
- helper functions;
- synthetic branches;
- adapters;
- validation machinery.

They MUST be considered implementation details unless a separate RFC defines otherwise.

#### 6.6.3 Source Identity

The canonical source identity belongs to Anuy.

Compiler/tooling MUST maintain the mapping:

```text
Anuy source span
    ↕
generated Go span
```

Go `//line`, DWARF metadata, and other backend mechanisms are projections of this canonical information.

They do not define the Anuy source model.

#### 6.6.4 Readability of Generated Go

Generated Go SHOULD be sufficiently structured for:

- diagnosing compiler bugs;
- troubleshooting interoperability;
- inspecting `anuy emit-go`.

It is not required to be idiomatic hand-written Go.

Human readability is a secondary goal after:

1. correctness;
2. stable interop representation;
3. source mapping;
4. toolchain compatibility.

### 6.7 Observability and Tooling

#### 6.7.1 Observability Is Part of Language Design

Runtime equivalence alone is insufficient for high-quality lowering.

Every feature SHOULD take into account:

- stepping;
- stack traces;
- variable visibility;
- coverage attribution;
- diagnostics;
- IDE navigation.

For example, a single Anuy expression MAY lower to several Go statements, but SHOULD remain one source-level operation where this matches the user model.

#### 6.7.2 User and Synthetic Operations

Compiler tooling MUST be able to distinguish:

```text
User
Synthetic
```

generated operations.

Synthetic operations may include:

- error checks;
- validation;
- temporary assignments;
- generated interface adapters;
- match dispatch;
- runtime helpers.

This distinction is used by tooling and MAY be used by optimization/debugging infrastructure.

#### 6.7.3 Coverage Philosophy

Anuy source coverage SHOULD reflect Anuy source semantics, not the number of executed lowered Go statements.

The basic v1 direction:

> **Coverage is measured in source-level executable operations/statements.**

Synthetic control flow SHOULD NOT automatically increase the coverage denominator.

#### 6.7.4 Debugging Philosophy

Anuy SHOULD reuse the Go debug ecosystem.

However, source-level stepping SHOULD correspond to Anuy semantics.

Example:

```text
var value = try operation()
next
```

SHOULD step to the next user Anuy operation, not show the programmer the generated error check.

#### 6.7.5 Progressive Tooling Principle

Anuy SHOULD reuse the existing tooling backend and add a bridge only where the generated Go representation differs from the source semantics.

Conceptually:

```text
Go compiler    → reuse
Go linker      → reuse
Go runtime     → reuse
Go modules     → reuse
gopls          → reuse through bridge
Delve          → reuse
Go cover       → reuse instrumentation

Anuy semantics → own
SourceMap      → own
CoverageMap    → own
LSP mapping    → own
```

#### 6.7.6 Language vs Toolchain Specification

The Language Specification MUST describe the observable language semantics.

It SHOULD NOT normatively depend on specific implementation technologies such as:

```text
-overlay
-toolexec
gopls
Delve
DWARF
```

These mechanisms belong to the Toolchain/Implementation Specification.

For example, the language can guarantee:

> A breakpoint refers to the `.anuy` source.

but is not required to normatively state:

> This is always implemented via DWARF `//line`.

### 6.8 Compatibility, Versioning and Performance

#### 6.8.1 Compatibility Philosophy

Anuy has several kinds of compatibility.

- **Runtime Compatibility** — Anuy uses the Go runtime.
- **Package Compatibility** — Anuy uses the Go package model.
- **Module Compatibility** — Anuy integrates with Go modules.
- **Library Compatibility** — Anuy imports Go packages.
- **Mixed Source Compatibility** — `.go` and `.anuy` can coexist.
- **API Compatibility** — Anuy can provide a Go-consumable API.
- **Tooling Compatibility** — Anuy integrates with existing Go tooling via bridges.

#### 6.8.2 Stability Before 1.0

Before Anuy 1.0, the language syntax, semantics, and compiler internals MAY change incompatibly.

However, pre-1.0 releases SHOULD:

- document breaking changes;
- have migration guidance where reasonable;
- avoid gratuitous churn;
- use the RFC process for substantial semantic changes.

#### 6.8.3 Versioning Principles

One should conceptually distinguish:

- the Anuy compiler version;
- the Anuy language version;
- the target Go version;
- the Go toolchain version;
- tooling/plugin versions.

They are not required to share the same numbering.

The specific versioning policy is defined by a separate RFC/tooling document.

#### 6.8.4 Runtime Support Budget

Any feature requiring Anuy runtime support MUST explicitly justify:

- why the helper/runtime is necessary;
- the runtime overhead;
- the impact on Go consumers;
- the versioning consequences;
- the compatibility consequences.

Zero-runtime lowering SHOULD be the preferred option.

#### 6.8.5 Performance Philosophy

Anuy SHOULD strive to have a performance profile close to that of the equivalent Go program.

Anuy does not guarantee:

> Any Anuy program is identical to hand-optimized Go.

But the language design SHOULD avoid non-obvious mandatory overheads.

Performance-sensitive differences SHOULD be documentable and predictable.

### 6.9 Design Filter and Feature Governance

#### 6.9.1 Design Filter for New Features

Every new proposal MUST explicitly answer:

- **Safety** — which class of errors becomes impossible or more explicit?
- **Simplicity** — what is the additional conceptual cost?
- **Go Lowering** — how is the feature expressed in Go?
- **Interoperability** — what does the Go caller see?
- **Foreign States** — which states can Go create but safe Anuy cannot?
- **Runtime Cost** — what hidden runtime cost appears?
- **Tooling** — how does the feature appear in the debugger, the IDE, and coverage?

A proposal SHOULD be rejected if its value is insufficient relative to these costs.

#### 6.9.2 Language Complexity Budget

Every new feature spends a limited complexity budget.

When evaluating it, one must take into account not only the parser/typechecker implementation, but also the impact on:

- interoperability;
- the generated representation;
- SourceMap;
- the debugger;
- coverage;
- the formatter;
- LSP;
- documentation;
- migration;
- the long-term teaching cost.

A small syntax feature with a large ecosystem cost MAY be rejected.

#### 6.9.3 RFC Requirement

Substantial language changes MUST go through the RFC process.

An RFC is required at least for changes to:

- the type system;
- initialization;
- interface semantics;
- error semantics;
- concurrency semantics;
- control flow;
- the public interop representation;
- the unsafe model;
- source compatibility;
- the fundamental toolchain contract.

Small bug fixes and uncontroversial grammar clarifications MAY be accepted without a full RFC according to project governance.

#### 6.9.4 RFC Evaluation Template

Every language RFC SHOULD include the following content (the document structure is fixed by `specs/rfcs/TEMPLATE.md`):

- **Motivation** — what problem does the proposal solve?
- **Goals** — which guarantees appear?
- **Non-goals** — what does the proposal deliberately not address?
- **Syntax** — how is the feature written?
- **Static Semantics** — what does the compiler check?
- **Runtime Semantics** — how is the construct executed?
- **Lowering** — how is it represented in Go?
- **Go Interoperability** — what happens at the Go ↔ Anuy boundary?
- **Foreign and Invalid States** — which states can come from Go?
- **Unsafe** — which assumptions must the programmer accept explicitly?
- **Source Mapping** — how are source spans mapped onto the generated representation?
- **Debug Observability** — how does the feature look under stepping/stack inspection?
- **Coverage Semantics** — which source-level units does the feature create?
- **Diagnostics** — which errors/warnings are expected?
- **Compatibility** — which compatibility risks exist?
- **Alternatives** — which alternatives were considered?
- **Open Questions** — what is not yet defined?

#### 6.9.5 Acceptance Criteria for Language Features

A feature SHOULD be accepted if:

1. it solves a clear correctness/reliability problem;
2. it matches the Go mental model or has a serious justification for deviating;
3. it has a reasonable lowering;
4. it does not break Go interop;
5. it has a clear foreign-state model;
6. it has acceptable complexity;
7. it has clear tooling behavior.

A feature SHOULD be rejected or deferred if:

- the benefit is mostly aesthetic;
- the implementation requires a heavy runtime;
- the Go representation becomes opaque;
- the feature creates many new special cases;
- safety improves marginally relative to the complexity;
- the feature makes gradual adoption substantially harder.

#### 6.9.6 Initial Language Direction

At the time of accepting RFC-000, the intended set of core features includes:

```text
Go-compatible core
+
non-null types by default
+
explicit nullable types
+
definite initialization without implicit defaults
+
explicit interface implementation
+
mandatory error handling
+
try error propagation
+
explicit discard
+
typed enums
+
exhaustive matching
+
no :=
+
unsafe for unverifiable invariants
```

This list is not the final normative feature set.

Each item must be accepted by its corresponding RFC.

#### 6.9.7 Success Criteria

Anuy can be considered successful if it makes it possible to create a codebase that:

- feels natural to a Go developer;
- has fewer representable invalid states;
- better expresses programmer intent;
- keeps access to the Go ecosystem;
- allows gradual migration;
- produces ordinary Go-compatible artifacts;
- does not require a separate complex runtime platform;
- keeps a high-quality debugging/IDE/testing experience.

#### 6.9.8 Anti-Goals as Warning Signs

The project should become wary if the evolution of the language leads to a situation where:

- generated Go is practically impossible to understand or use;
- most of the Go API requires handwritten adapters;
- the standard library has to be duplicated;
- a new runtime becomes mandatory for most features;
- the language manual grows mostly out of special cases;
- debugging regularly shows generated code;
- an ordinary Go developer stops recognizing the execution model;
- the Anuy ecosystem becomes isolated from Go.

Such signs require a revision of the design direction.

## 7. Interaction with Other RFCs

RFC-000 is the primary design filter of the project: all RFCs in the set are reviewed against its normative principles (see the Decision Record and Section 6.9). The delegation of foundational questions is in Section 12.

### 7.1 RFC Roadmap

After RFC-000, the following order is expected:

```text
RFC-001 — Values, Initialization and Trust
RFC-002 — Nullability
RFC-003 — Variables, Assignment and Scope
RFC-004 — Interfaces and Explicit impl
RFC-005 — Error Handling
RFC-006 — Enums and Exhaustive Matching
RFC-007 — Unsafe and Foreign Contracts
RFC-008 — Go Interoperability
RFC-009 — Lowering, Source Identity and Generated Go Contract
RFC-010 — Build, Test and Publish Model
RFC-011 — Diagnostics, Debugging, Coverage and Tooling
RFC-012 — Sum Types
```

The order MAY change, but the foundational semantics SHOULD be designed before the dependent features.

## 9. Rationale

### 9.1 Consequences

Accepting this RFC means that future proposals are not evaluated in isolation.

For example, a proposal to add a new feature must take into account not only:

```text
can this be implemented?
```

but also:

```text
does it improve correctness?
how much does it complicate the language?
how does it lower?
how does Go see it?
how is it debugged?
how is it covered by tests?
which invalid states appear at the foreign boundary?
```

RFC-000 is the primary design filter of the project.

## 12. Open Questions

This RFC deliberately does not fix:

- the full Language Core grammar, including composite nullable types;
- the enum representation;
- the foreign validation policy;
- the shadowing lint policy;
- the runtime-support package;
- the versioning model;
- the target Go support policy.

Initialization is defined by RFC-001, nullability by RFC-002, shadowing by RFC-003, `impl` by RFC-004, and `try` by RFC-005. The remaining foundational grammar questions belong to a future Language Core RFC/reference manual.

## 14. References

### 14.1 Normative

None: RFC-000 is the root document of the RFC set and does not rely on other normative documents.

### 14.2 Informative

- [RFC index and governance](README.md) — authority order, lifecycle, deferred foundations.
