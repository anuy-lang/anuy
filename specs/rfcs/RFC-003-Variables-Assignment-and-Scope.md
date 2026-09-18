# RFC-003 — Variables, Assignment and Scope

**Status:** Accepted
**RFC:** 003
**Title:** Variables, Assignment and Scope
**Language:** Anuy
**Area:** Variables / Assignment / Scope / Shadowing / Loops / Closures
**Version:** 4
**Date:** 2026-09-19
**Requires:** RFC-000, RFC-001, RFC-002, RFC-004, RFC-005, RFC-006, RFC-007, RFC-008, RFC-009, RFC-011
**Supersedes:** —
**Canonical:** English (public repository)

---

## Decision Record

RFC-003 reviewed against RFC-001 preserves initialization state per binding; reviewed against RFC-002, it preserves nullability refinement per binding identity.

Section 12 assigns every remaining question before `Accepted` to its responsible RFC, Language Core work or this RFC before `Accepted`.

The project owner approved RFC-003 as Proposed.

2026-09-19: published as the canonical English text in the public repository (owner decision); section numbering and normative content unchanged; Version 3 → 4.

---

## 1. Abstract

This RFC defines the model of local variables: declaration and assignment are distinct syntactic operations (`var` and `=`), the Go short declaration `:=` is absent, `var` always creates a new binding in the current lexical scope, and `=` modifies the nearest visible assignable binding. Ordinary lexical shadowing is allowed, redeclaration in the same scope is forbidden, and suspicious shadowing remains a lint concern. The model covers multiple declaration/assignment, control-flow joins, loops, closure capture, and interactions with definite initialization (RFC-001) and nullability refinement (RFC-002).

## 2. Solutions

> **Declaration and assignment are distinct syntactic operations.**

> **`var` always creates a new binding in the current lexical scope.**

> **`=` never creates a binding and always modifies the nearest visible assignable binding.**

> **Redeclaration in the same scope is forbidden; lexical shadowing is allowed.**

> **Potentially dangerous shadowing is a lint concern, not a language error.**

> **Anuy removes ambiguity between declaration and assignment, not the standard lexical-scoping model itself.**

## 3. Motivation

The Go short declaration:

```go
x, err := operation()
```

can at the same time:

- declare new bindings;
- reuse existing bindings.

Meaning depends on the surrounding scope.

This creates unwanted contextual semantics.

For example:

```go
x, err := operation()
```

can mean:

```text
x   = existing binding
err = new binding
```

or:

```text
x   = new binding
err = new binding
```

depending on which names already exist in the current block.

Anuy eliminates exactly this ambiguity:

```text
var → declaration
=   → assignment
```

At the same time, ordinary lexical shadowing by itself is not the same problem.

In:

```anuy
var value = raw()

if normalize {
    var value = normalizeValue(value)
}
```

the programmer explicitly wrote `var`, that is, explicitly requested a new binding.

Therefore Anuy allows lexical shadowing while preserving explicit declaration semantics.

## 4. Goals

The RFC defines:

- `var`;
- type inference;
- declaration without initializer;
- assignment;
- first assignment;
- reassignment;
- multiple declaration;
- multiple assignment;
- prohibition mixed declare/reassign;
- lexical scopes;
- lexical shadowing;
- same-scope redeclaration;
- name resolution;
- branch-local variables;
- definite initialization across control flow;
- nullability refinement;
- loops;
- closure capture;
- assignment evaluation order;
- lint guidance for suspicious shadowing.

## 5. Non-goals

This RFC does not fully define:

- immutable bindings;
- `const`;
- destructuring patterns;
- ownership;
- move semantics;
- borrow checking;
- exact loop syntax;
- exact `range` syntax;
- constructors;
- full error handling;
- lint configuration format;
- compiler effect system.

## 6. Specification

### 6.1 Binding, Declaration and Assignment

#### 6.1.1 Binding

A **binding** associates a source name with a distinct variable identity.

Example:

```anuy
var count int
```

creates one binding named:

```text
count
```

Internally compiler SHOULD identify it through stable semantic identity such as:

```text
SymbolID
```

Name strings alone are insufficient because multiple shadowing bindings may share the same source name.

#### 6.1.2 Declaration

A declaration introduces a new binding in the **current lexical scope**.

All ordinary local declarations use:

```text
var
```

#### 6.1.3 Assignment

Assignment changes the value of an already existing binding or performs its first initialization.

Example:

```anuy
count = 42
```

Assignment never introduces a name.

#### 6.1.4 Core Syntax

Anuy v1 supports:

```anuy
var x = expression
```

```anuy
var x T = expression
```

```anuy
var x T
```

and:

```anuy
x = expression
```

#### 6.1.5 No `:=`

Anuy does not support:

```anuy
x := expression
```

nor:

```anuy
x, y := expression
```

Use of `:=` MUST be a syntax error.

#### 6.1.6 Declaration With Type Inference

```anuy
var x = expression
```

creates a new binding in the current scope.

Its static type is inferred from `expression`.

Example:

```anuy
var count = 42
```

conceptually gives:

```text
count: int
```

#### 6.1.7 Explicit Type

```anuy
var x T = expression
```

creates `x` with declared type `T`.

RHS MUST be assignable to `T`.

#### 6.1.8 Declaration Without Initializer

```anuy
var x T
```

creates an uninitialized binding.

RFC-001 definite-initialization rules apply.

#### 6.1.9 Inference Requires Initializer

Invalid:

```anuy
var x
```

Compiler cannot infer a type.

Programmer must write:

```anuy
var x T
```

or:

```anuy
var x = expression
```

#### 6.1.10 Assignment Resolves Existing Binding

```anuy
x = expression
```

resolves the nearest visible assignable binding named `x`.

If none exists, compilation fails.

Recommended diagnostic — Section 8.4 (D-1).

#### 6.1.11 First Assignment

For:

```anuy
var x T
```

the first:

```anuy
x = expression
```

performs:

```text
Uninitialized
→
Initialized
```

No separate initialization operator exists.

#### 6.1.12 Reassignment

After initialization:

```anuy
x = expression
```

is ordinary mutation.

Example:

```anuy
var count int
count = 1
count = 2
```

valid.

#### 6.1.13 Fundamental Declaration Rule

```anuy
var x = expression
```

always means:

> Create a new binding named `x` in the current lexical scope.

It MUST NOT silently become assignment because an outer `x` exists.

#### 6.1.14 Fundamental Assignment Rule

```anuy
x = expression
```

always means:

> Assign the nearest visible existing binding named `x`.

It MUST NOT implicitly create a variable.

### 6.2 Multiple Declaration and Assignment

#### 6.2.1 Multiple Declaration

Anuy supports:

```anuy
var x, y = operation()
```

All listed names are declarations in the current scope.

Therefore all names MUST be absent from the **current scope**.

They MAY shadow names from outer scopes.

#### 6.2.2 Multiple Assignment

Anuy supports:

```anuy
x, y = operation()
```

Every target MUST already resolve to an assignable binding.

Targets may resolve to bindings from the current or outer lexical scopes.

#### 6.2.3 No Mixed Declaration and Assignment

Anuy does not have a statement where one identifier is declared while another is reassigned.

Given:

```anuy
var x = 1
```

this:

```anuy
var x, y = operation()
```

is rejected because `x` is already declared in the current scope.

This:

```anuy
x, y = operation()
```

is rejected if `y` resolves to no existing binding.

#### 6.2.4 Shadowing Does Not Make Multiple Declaration Mixed

Given:

```anuy
var x = 1

if condition {
    var x, y = operation()
}
```

both inner `x` and `y` are new bindings in the inner scope.

The outer `x` is shadowed.

This is valid.

#### 6.2.5 Why No Mixed Form

Statement meaning must not depend on whether some arbitrary LHS name already exists.

Anuy preserves:

```text
var → every listed binding is new in this scope
=   → every listed target already exists
```

#### 6.2.6 Multiple Assignment Evaluation

RHS values are evaluated before any LHS updates become observable.

Therefore:

```anuy
x, y = y, x
```

performs parallel swap semantics.

#### 6.2.7 Swap

Valid when both bindings initialized:

```anuy
x, y = y, x
```

If an RHS binding may be uninitialized, compilation fails.

#### 6.2.8 Duplicate Assignment Targets

Example:

```anuy
x, x = 1, 2
```

SHOULD be rejected.

It has little legitimate use and creates unnecessary order sensitivity.

#### 6.2.9 First Assignment Cannot Read Binding

Invalid:

```anuy
var x int
x = x + 1
```

RHS reads uninitialized `x`.

#### 6.2.10 Blank Identifier

Anuy MAY retain:

```text
_
```

for discard of ordinary values.

Example:

```anuy
var value, _ = operation()
```

`_` does not create a binding.

RFC-005 MAY prohibit or restrict `_` for `error?` in favor of explicit `discard`.

#### 6.2.11 Outer Binding Does Not Prevent Multiple Declaration

This is valid:

```anuy
var x = 1

{
    var x, y = operation()
}
```

because `x` is new in the inner scope.

### 6.3 Lexical Scope and Shadowing

#### 6.3.1 Lexical Scope

Anuy uses ordinary lexical scoping.

Scopes are established by constructs including:

- function bodies;
- blocks;
- branches;
- loops;
- closures;
- other constructs explicitly defined to introduce a scope.

#### 6.3.2 Block Scope

A block:

```anuy
{
    ...
}
```

creates a lexical scope.

Example:

```anuy
{
    var x = 1
    print(x)
}

print(x) // error
```

#### 6.3.3 Branch Scope

Each branch has its own lexical scope.

```anuy
if condition {
    var x = 1
}

print(x) // error
```

#### 6.3.4 Loop Scope

Loop bodies create lexical scopes.

Iteration bindings belong to the loop construct according to loop rules.

#### 6.3.5 Same-scope Redeclaration Is Forbidden

Within one lexical scope a source binding name may be declared only once.

Invalid:

```anuy
var x = 1
var x = 2
```

Diagnostic — Section 8.4 (D-2).

#### 6.3.6 Same-scope Multiple Redeclaration

Invalid:

```anuy
var x = 1
var x, y = operation()
```

because `x` already exists in the current scope.

Diagnostic — Section 8.4 (D-4).

#### 6.3.7 Lexical Shadowing Is Allowed

A declaration MAY use a name already visible from an outer lexical scope.

Example:

```anuy
var x = 1

if condition {
    var x = 2
    print(x)
}

print(x)
```

The two declarations represent distinct bindings.

#### 6.3.8 Shadowing Is Explicit Through `var`

No separate syntax such as:

```anuy
shadow var x = ...
```

exists.

The declaration keyword itself communicates intent:

```anuy
var x = ...
```

means new binding.

```anuy
x = ...
```

means existing binding.

This distinction is considered sufficient.

#### 6.3.9 Name Resolution

A reference to:

```anuy
x
```

resolves to the nearest visible binding with that name.

This applies to:

- reads;
- assignments;
- address-taking;
- captures.

#### 6.3.10 Shadowing Example

```anuy
var value = 1

{
    var value = 2

    {
        var value = 3
        print(value) // 3
    }

    print(value) // 2
}

print(value) // 1
```

Each declaration has a separate semantic identity.

#### 6.3.11 Assignment Under Shadowing

```anuy
var value = 1

{
    var value = 2
    value = 3
}

print(value)
```

The assignment changes only the inner binding.

Outer `value` remains `1`.

#### 6.3.12 Assignment Without Inner Shadow

```anuy
var value = 1

{
    value = 2
}

print(value)
```

The assignment resolves to the outer binding and changes it to `2`.

#### 6.3.13 Scope of a Newly Declared Binding

A newly declared binding is **not visible inside its own initializer**.

Its scope begins after the declaration has completed.

This is important for shadowing.

#### 6.3.14 Transformational Shadowing

Example:

```anuy
var value = rawValue()

if normalize {
    var value = normalizeValue(value)
    consume(value)
}
```

Inside:

```anuy
normalizeValue(value)
```

the `value` reference resolves to the outer binding.

After declaration completes, the inner `value` shadows it.

This pattern is valid and intentional.

#### 6.3.15 No Self-reference Through New Binding

Without an outer binding:

```anuy
var x = x
```

is invalid because no visible `x` exists inside the initializer.

With an outer binding:

```anuy
var x = 1

{
    var x = x + 1
}
```

the initializer reads the outer `x`.

#### 6.3.16 `if` Scope

Bindings declared before an `if` remain visible in its branches.

Branch-local declarations remain local to their branch.

#### 6.3.17 No Go-style `if` Initializer in v1

Go:

```go
if x := f(); x != nil {
    ...
}
```

is not required in Anuy v1.

Preferred:

```anuy
var x = f()

if x != nil {
    ...
}
```

This keeps declaration syntax uniform.

#### 6.3.18 Rationale

Special declaration positions complicate otherwise simple scope rules.

The extra source line is considered acceptable.

### 6.4 Parameters and Results

#### 6.4.1 Parameters

Function parameters are initialized bindings in an enclosing function scope.

A declaration in the same effective scope cannot redeclare them.

Example conceptually invalid:

```anuy
func process(user User) {
    var user = loadOther()
}
```

if the function body is the same lexical scope as its parameters.

#### 6.4.2 Nested Parameter Shadowing

A nested block MAY shadow a parameter:

```anuy
func process(user User) {
    if normalize {
        var user = normalizeUser(user)
        save(user)
    }
}
```

This is ordinary lexical shadowing.

Suspicious cases MAY produce lint warnings.

#### 6.4.3 Closure Parameters

Closure parameters MAY shadow outer locals:

```anuy
var user = currentUser()

users.forEach(func(user User) {
    print(user)
})
```

This is legal.

It is often natural because callback parameter names describe the role within the callback.

#### 6.4.4 Parameter Reassignment

Parameters are initialized at function entry.

Baseline v1 permits parameter reassignment:

```anuy
func normalize(value int) int {
    value = abs(value)
    return value
}
```

unless future immutability RFC changes this.

#### 6.4.5 Nested Shadowing of Parameters

Legal:

```anuy
func process(value Value) {
    if normalize {
        var value = normalizeValue(value)
        consume(value)
    }
}
```

Initializer reads parameter; after declaration the inner binding shadows it.

#### 6.4.6 Named Results

If named results exist, they are ordinary bindings subject to definite initialization.

They MUST NOT gain implicit Go zero initialization semantics.

#### 6.4.7 Bare Return

If bare return exists, every required named result binding must be definitely initialized.

Exact named-result design remains open.

### 6.5 Closures

#### 6.5.1 Closures

Closures capture bindings by semantic identity.

Ordinary mutable capture refers to the existing binding rather than a snapshot value, except where iteration rules define per-iteration identity.

#### 6.5.2 Capture Example

```anuy
var count = 0

var increment = func() {
    count = count + 1
}
```

Closure captures outer `count`.

#### 6.5.3 Shadowing Inside Closure

```anuy
var count = 0

var f = func() {
    var count = 100
    print(count)
}
```

The inner declaration shadows outer `count`.

Outer binding is not captured merely because names match.

#### 6.5.4 Capture Before Initialization

RFC-001 baseline remains:

If closure reads a captured binding, that binding MUST be definitely initialized when closure is created.

Invalid:

```anuy
var service Service

var callback = func() {
    service.Run()
}

service = createService()
```

#### 6.5.5 Write-only Closure Capture

A closure MAY assign an outer uninitialized binding if it does not read it:

```anuy
var x int

var initialize = func() {
    x = 1
}
```

However arbitrary invocation of that closure does not automatically establish definite initialization for compiler dataflow.

#### 6.5.6 Calls Do Not Establish Caller Initialization

```anuy
initialize()
print(x)
```

remains rejected under baseline v1 if `x` was otherwise uninitialized.

General interprocedural initialization effects are out of scope.

#### 6.5.7 Closure Mutation and Null Refinement

```anuy
var user User? = findUser()

var clear = func() {
    user = nil
}

if user != nil {
    clear()
    user.save()
}
```

`user.save()` must be rejected if compiler sees that `clear()` may mutate the same binding.

### 6.6 Loops

#### 6.6.1 Loops

Anuy retains a small Go-like loop model.

At minimum:

- condition loops;
- infinite loops;
- iteration loops.

The exact grammar (owner decision 2026-09-14; see Section 12.1):

```ebnf
LoopStatement     = "for" [ Expression ] Block
                  | "for" identifier "in" Expression Block ;
BreakStatement    = "break" ;
ContinueStatement = "continue" ;
```

- `for <expression> { … }` is a condition loop: the body may execute zero
  times (Sections 6.6.2–6.6.3).
- `for { … }` is an infinite loop: it exits only through `break` (Sections 6.6.4–6.6.5).
- `for <identifier> in <expression> { … }` is an iteration loop: the binding
  is created initialized for each logical iteration (Section 6.6.9) and belongs to the
  loop scope (Section 6.3.4); it may shadow an outer binding (Sections 6.7.1, 6.6.10, 13.20).
- `break` exits the nearest enclosing loop; `continue` starts the next
  iteration of the nearest enclosing loop (Sections 6.6.4–6.6.6). Labeled transfer is not
  part of the grammar.
- `in` is a keyword only in the loop header, between the binding and the
  collection expression.

Range- or index-style iteration forms (`for i, item in items`) and labeled
transfer remain assigned to Language Core (Section 12.1).

#### 6.6.2 Conditional Loop Does Not Guarantee Execution

```anuy
var x int

for condition {
    x = 1
}

print(x)
```

is rejected because loop may execute zero times.

#### 6.6.3 Initialized Before Loop

```anuy
var x = 0

for condition {
    x = update(x)
}

print(x)
```

valid.

#### 6.6.4 Infinite Loop and Break

```anuy
var x int

for {
    x = readValue()

    if valid(x) {
        break
    }
}

print(x)
```

may establish initialization because every reachable exit assigns `x`.

#### 6.6.5 Invalid Break Path

```anuy
var x int

for {
    if cancel() {
        break
    }

    x = readValue()

    if valid(x) {
        break
    }
}

print(x)
```

rejected because one reachable loop exit leaves `x` uninitialized.

#### 6.6.6 Continue

`continue` creates a back edge to the next iteration.

Only reachable loop exits contribute to post-loop state.

#### 6.6.7 Loop-carried Initialization

Uninitialized variables must be initialized before each read on every iteration.

Invalid:

```anuy
var x int

for condition {
    print(x)
    x = 1
}
```

First iteration may read before initialization.

#### 6.6.8 Iteration Bindings

Conceptually:

```anuy
for item in items {
    ...
}
```

creates an initialized binding `item` for each logical iteration.

#### 6.6.9 Per-iteration Identity

Iteration bindings SHOULD have per-iteration logical identity for closure capture.

Example:

```anuy
for user in users {
    callbacks.append(func() {
        print(user.name)
    })
}
```

Each closure should observe its corresponding iteration's `user`.

#### 6.6.10 Iteration Shadowing

This is legal:

```anuy
var user = defaultUser()

for user in users {
    process(user)
}
```

Loop `user` shadows outer `user`.

Tooling MAY lint it if deemed suspicious.

### 6.7 Shadowing Beyond Locals

#### 6.7.1 Loop Variables

Iteration variables MAY shadow outer bindings:

```anuy
var item = defaultItem()

for item in items {
    process(item)
}
```

This is legal lexical shadowing.

Lint MAY warn depending on policy, but language MUST NOT reject it solely for shadowing.

#### 6.7.2 Package-level Variables

A local declaration MAY shadow a package-level binding.

Example:

```anuy
var config = loadConfig()

func test() {
    var config = testConfig()
    use(config)
}
```

This is legal.

Because package/local shadowing is potentially confusing, lint SHOULD normally report it unless explicitly suppressed/configured.

#### 6.7.3 Imports

Local scopes MAY shadow imported package names if ordinary lexical rules permit it.

Example conceptually:

```anuy
import "time"

func f() {
    var time = readTimeValue()
}
```

Language MAY allow this.

Tooling SHOULD consider package-name shadowing suspicious.

Exact import namespace rules belong to RFC-008/module specification.

#### 6.7.4 Member Names

Fields and methods do not participate in ordinary lexical shadowing.

This is valid:

```anuy
var name = input
user.name = name
```

because `user.name` is qualified member access.

#### 6.7.5 Type Namespace

Whether type names and value names use one or multiple namespaces is defined elsewhere.

This RFC's shadowing rules concern value bindings.

#### 6.7.6 Package-level Bindings

RFC-001 requires package-level variables to have initializers.

Example:

```anuy
var config = loadConfig()
```

is initialized before ordinary package use according to package initialization semantics.

#### 6.7.7 Local Shadowing of Package Binding

Legal:

```anuy
var config = loadConfig()

func test() {
    var config = testConfig()
    use(config)
}
```

Lint SHOULD normally flag such cases because package state may be unintentionally hidden.

### 6.8 Assignment Targets and Operations

#### 6.8.1 Assignment Targets

Assignment target MAY include:

- local variable;
- field;
- index;
- dereference;

subject to relevant type and safety rules.

#### 6.8.2 Field Assignment Through Nullable Receiver

Invalid without narrowing:

```anuy
var user User? = findUser()
user.name = "Alice"
```

Valid:

```anuy
if user != nil {
    user.name = "Alice"
}
```

#### 6.8.3 Safe Navigation Is Not Assignment Target

Anuy v1 does not define:

```anuy
user?.name = "Alice"
```

Programmer writes explicit control flow:

```anuy
if user != nil {
    user.name = "Alice"
}
```

#### 6.8.4 Compound Assignment

If supported:

```anuy
x += y
```

performs both read and write.

Therefore `x` MUST already be initialized.

#### 6.8.5 Increment and Decrement

If retained:

```anuy
x++
x--
```

they require initialized numeric target.

They cannot initialize an uninitialized binding.

#### 6.8.6 Assignment Type

Assignment always checks against the **declared type**, not temporary flow-refined type.

Example:

```anuy
var user User? = findUser()

if user != nil {
    user = nil // valid
}
```

Declared type remains `User?`.

The assignment simply invalidates the refinement.

#### 6.8.7 Address Taking

Taking address of an initialized binding MAY be allowed.

Taking address of an uninitialized binding is forbidden by RFC-001 baseline because foreign/aliased code could observe backend zero representation.

#### 6.8.8 Out Parameters

Anuy MUST NOT infer initialization merely because address of a variable is passed to foreign code.

Dedicated Go interop rules may define controlled out-parameter patterns.

### 6.9 Binding Identity: Initialization, Refinement and Dataflow

#### 6.9.1 Definite Initialization Uses Binding Identity

Shadowed bindings are analyzed independently.

Example:

```anuy
var x int

if condition {
    var x = 10
    print(x)
}

print(x)
```

The inner initialized `x` does not initialize the outer `x`.

Final read is an error.

#### 6.9.2 Branch Initialization

Valid:

```anuy
var x int

if condition {
    x = 1
} else {
    x = 2
}

print(x)
```

Both branches assign the same outer binding.

#### 6.9.3 Branch Shadowing Does Not Initialize Outer Binding

Invalid:

```anuy
var x int

if condition {
    var x = 1
} else {
    var x = 2
}

print(x)
```

Each branch declares a different local `x`.

Outer `x` remains uninitialized.

#### 6.9.4 Early Exit Initialization

Valid:

```anuy
var value Value

if invalid {
    return
}

value = createValue()
use(value)
```

Only continuing paths matter.

#### 6.9.5 Nullability Refinement Uses Binding Identity

Example:

```anuy
var user User? = findUser()

if user != nil {
    var user = normalizeUser(user)
    save(user)
}
```

The inner `user` is a different binding.

Outer nullability state remains conceptually separate.

#### 6.9.6 Refinement of Outer Binding

```anuy
var user User? = findUser()

if user == nil {
    return
}

save(user)
```

continuing path refines the outer binding to `User`.

#### 6.9.7 Shadowed Assignment Does Not Affect Outer Refinement

```anuy
var user User? = findUser()

if user != nil {
    var user User? = nil
    // inner user is nil
}

// outer user retains whatever refinement is valid
```

Flow analysis is keyed by binding identity.

#### 6.9.8 Assignment Invalidates Refinement

For one binding:

```anuy
var user User? = findUser()

if user != nil {
    user = findOtherUser()
    save(user)
}
```

If `findOtherUser()` returns `User?`, previous refinement is lost.

#### 6.9.9 Compiler Dataflow

Compiler SHOULD use CFG-based analysis tracking at least:

```text
DefiniteInitialization(SymbolID)
NullabilityRefinement(SymbolID)
```

Bindings that share textual names remain independent through `SymbolID`.

#### 6.9.10 Scope and Dataflow Are Separate

Name resolution answers:

```text
Which binding does this identifier refer to?
```

Dataflow answers:

```text
Is that binding initialized?
Is it currently known to be non-nil?
```

Shadowing affects the first question, not the fundamental model of the second.

#### 6.9.11 Source Identity

Every declaration receives stable semantic identity.

For:

```anuy
var user = first()

{
    var user = second()
}
```

compiler conceptually sees:

```text
user#17
user#23
```

not one mutable name.

### 6.10 Source Mapping, Debugging and Coverage

#### 6.10.1 Source Mapping

Every declaration has its own source identity / `SymbolID`.

Two shadowing variables with the same name MUST remain distinguishable in:

- semantic analysis;
- SourceMap;
- LSP;
- debugger;
- references/rename.

#### 6.10.2 Declaration Without Initializer

```anuy
var x T
```

is a source declaration but not necessarily an executable coverage operation.

#### 6.10.3 Declaration With Initializer

```anuy
var x = expression
```

is one source-level declaration+initialization operation.

Generated Go MAY split storage and assignment, but source tooling should preserve the Anuy construct.

#### 6.10.4 Assignment

```anuy
x = expression
```

is a user operation attributed to that assignment.

#### 6.10.5 Synthetic Temporaries

Compiler MAY generate temporary variables for:

- multiple assignment;
- safe navigation;
- evaluation-order preservation;
- lowering.

They are synthetic and MUST NOT participate in source lexical lookup.

#### 6.10.6 Debugging

Debugger MUST distinguish shadowed source variables by lexical scope and semantic identity.

When stopped inside:

```anuy
var x = 1

{
    var x = 2
    // stop
}
```

the inner `x` is the currently visible `x`.

Tooling MAY still expose outer bindings through advanced scope inspection.

#### 6.10.7 Debugging Uninitialized Variables

Before initialization, source debugger SHOULD:

- hide binding; or
- display `<uninitialized>`.

It MUST NOT display backing Go zero representation as a valid source value.

#### 6.10.8 Coverage

Pure declaration:

```anuy
var x T
```

without executable initializer is not an independent coverage unit.

#### 6.10.9 Initialized Declaration Coverage

```anuy
var x = expression
```

is an executable source operation.

#### 6.10.10 Assignment Coverage

```anuy
x = expression
```

is executable.

#### 6.10.11 Synthetic Temporaries and Coverage

Generated variables/moves introduced for lowering MUST NOT inflate source coverage.

### 6.11 Compatibility and Migration

#### 6.11.1 Compatibility With Go

Anuy retains familiar lexical shadowing from Go:

```anuy
var x = 1

if condition {
    var x = 2
}
```

But removes Go's short declaration ambiguity.

Go:

```go
x, err := operation()
```

Anuy requires either declaration:

```anuy
var x, err = operation()
```

or assignment:

```anuy
x, err = operation()
```

No statement can mean both.

#### 6.11.2 Migration From Go

Migration tooling SHOULD classify every Go `:=` occurrence into:

- all-new declaration;
- mixed declaration/reassignment;
- shadowing declaration.

All-new case can become:

```anuy
var ...
```

Mixed case requires explicit restructuring.

Ordinary nested shadowing MAY remain shadowing in Anuy.

#### 6.11.3 Go Shadowing Migration

Example Go:

```go
x := outer()

if condition {
    x := inner()
    use(x)
}
```

can become directly:

```anuy
var x = outer()

if condition {
    var x = inner()
    use(x)
}
```

No forced rename is necessary.

#### 6.11.4 Reliability

This RFC eliminates several ambiguity classes:

- accidental declaration via assignment syntax;
- mixed new/existing short declarations;
- same-scope redeclaration;
- implicit variable creation.

It does **not** attempt to eliminate lexical shadowing itself.

Instead potentially risky cases are delegated to static analysis.

#### 6.11.5 Conformance Tests

Minimum suite MUST include:

```text
inferred declaration
typed declaration
uninitialized declaration
unknown assignment
reassignment
multiple declaration
multiple assignment
mixed declaration rejection
same-scope redeclaration rejection
nested lexical shadowing
assignment to nearest binding
initializer reads outer shadowed binding
parameter nested shadowing
closure parameter shadowing
package binding shadowing
branch-local scope
outer initialization unaffected by shadow
branch initialization
early-return initialization
loop zero iterations
loop break initialization
loop variable shadowing
per-iteration closure capture
nullable refinement
shadowed nullable bindings
refinement invalidation
closure read before initialization
closure mutation
address-taking uninitialized value
debugger lexical scope
rename with shadowing
lint suspicious shadowing
coverage declaration vs assignment
```

### 6.12 Examples

#### 6.12.1 Declaration

```anuy
var user = loadUser()
```

#### 6.12.2 Deferred Initialization

```anuy
var user User

if cached {
    user = cachedUser()
} else {
    user = try loadUser()
}

process(user)
```

#### 6.12.3 Explicit Assignment to Outer Binding

```anuy
var result = initial()

if condition {
    result = recompute()
}

use(result)
```

#### 6.12.4 Explicit Shadowing

```anuy
var result = initial()

if condition {
    var result = recompute()
    use(result)
}

use(result)
```

The difference from the previous example is visible solely through presence of `var`.

#### 6.12.5 Transformational Shadowing

```anuy
var input = read()

if normalize {
    var input = normalizeInput(input)
    process(input)
}
```

The initializer reads outer `input`.

#### 6.12.6 Multiple Declaration

```anuy
var value, err = operation()
```

Both names are new in current scope.

#### 6.12.7 Multiple Assignment

```anuy
var value Value
var err error?

value, err = operation()
```

Both targets resolve to existing bindings.

#### 6.12.8 Inner Multiple Declaration

```anuy
var value = fallback()

if condition {
    var value, err = operation()
    handle(value, err)
}
```

Inner `value` shadows outer `value`.

#### 6.12.9 Branch-local Bindings

```anuy
if enabled {
    var message = "enabled"
    print(message)
}

print(message) // error
```

#### 6.12.10 Branch Merge Through Outer Binding

```anuy
var message string

if enabled {
    message = "enabled"
} else {
    message = "disabled"
}

print(message)
```

#### 6.12.11 Branch Shadow Does Not Merge

```anuy
if enabled {
    var message = "enabled"
} else {
    var message = "disabled"
}

print(message) // error
```

The two declarations are unrelated branch-local bindings.

#### 6.12.12 Nullable Refinement

```anuy
var user User? = findUser()

if user == nil {
    return
}

save(user)
```

#### 6.12.13 Shadowed Nullable

```anuy
var user User? = findUser()

if user != nil {
    var user = normalizeUser(user)
    save(user)
}
```

Initializer consumes narrowed outer `User`.

Inner `user` is a separate binding inferred from `normalizeUser`.

#### 6.12.14 Loop Shadowing

```anuy
var item = defaultItem()

for item in items {
    process(item)
}

use(item)
```

The loop binding does not modify outer `item`.

#### 6.12.15 Loop Assignment

```anuy
var item = defaultItem()

for next in items {
    item = next
}

use(item)
```

Here outer `item` is explicitly mutated.

#### 6.12.16 Closure Parameter Shadowing

```anuy
var user = currentUser()

users.forEach(func(user User) {
    send(user)
})
```

Legal.

#### 6.12.17 Closure Capture

```anuy
var count = 0

var increment = func() {
    count = count + 1
}
```

No inner `count` exists, so closure captures outer binding.

#### 6.12.18 Closure Shadowing

```anuy
var count = 0

var show = func() {
    var count = 100
    print(count)
}
```

Closure does not capture outer `count` for this reference.

## 7. Interaction with Other RFCs

### 7.1 RFC-001

RFC-001 defines:

```text
Uninitialized
Initialized
```

per binding.

Shadowed variables are separate bindings and therefore have independent initialization state.

### 7.2 RFC-002

RFC-002 defines nullable types and flow narrowing.

Refinement is tracked per binding identity.

A shadowing declaration neither inherits nor modifies outer binding state except through values explicitly read in its initializer.

### 7.3 RFC-004

Methods/interface implementations use the same lexical binding rules.

Method receivers/parameters participate in lexical scope according to their declaration scopes.

### 7.4 RFC-005

`try` creates a failure path that exits the current function.

Definite initialization only considers continuing success path after:

```anuy
var value = try operation()
```

Exact semantics belong to RFC-005.

### 7.5 RFC-006

An exhaustive `match` may establish definite initialization of an outer binding if every continuing arm assigns that same binding.

Arm-local shadowing bindings do not count as assignments to the outer binding.

### 7.6 RFC-007

`unsafe` does not alter scope or declaration semantics.

It cannot make undeclared identifiers assignable or bypass same-scope redeclaration rules.

### 7.7 RFC-008

Interop RFC must define effects of:

- foreign callbacks;
- output parameters;
- address-taking;
- imported identifiers.

Ordinary lexical shadowing remains unchanged.

### 7.8 RFC-009

Lowering MUST preserve:

- lexical name resolution;
- binding identity;
- assignment evaluation order;
- parallel multiple assignment;
- per-iteration loop semantics;
- source mapping of shadowed variables.

Generated Go names MAY be mangled to avoid backend collisions.

### 7.9 RFC-011

Tooling RFC SHOULD define:

- shadowing lint;
- lexical scope visualization;
- hover binding identity;
- rename conflicts;
- debugger visibility;
- suspicious outer-variable usage diagnostics.

## 8. Diagnostics and Tooling

### 8.1 Rename and References

#### 8.1.1 Rename Refactoring

Rename uses binding identity, not textual name matching.

Renaming one shadowed `x` MUST NOT rename another unrelated `x`.

#### 8.1.2 Rename and Collisions

If rename would create a same-scope redeclaration, LSP MUST reject or report conflict.

If rename merely creates legal lexical shadowing, LSP MAY allow it but SHOULD surface a shadowing warning.

#### 8.1.3 Find References

Find-references MUST resolve references by `SymbolID`.

Shadowed variables with the same name have separate reference sets.

### 8.2 Shadowing Lint

#### 8.2.1 Suspicious Shadowing Is a Lint Concern

Although shadowing is legal, compiler tooling SHOULD offer analysis for likely mistakes.

Potential lint cases include:

- shadowing package-level bindings;
- shadowing imported package names;
- shadowing `err`;
- outer binding used immediately after inner scope;
- similar names combined with control-flow exits;
- shadowing parameters in complex functions.

These are recommendations, not language errors.

#### 8.2.2 Lint Must Not Define Language Semantics

A program containing legal shadowing remains valid Anuy even if lint reports it.

Lint policy MAY be:

- enabled by default;
- configurable;
- promoted to error by project settings.

But promotion is a tooling/project policy, not core language semantics.

#### 8.2.3 Typical Suspicious Example

```anuy
var err error? = nil

if condition {
    var err = operation()
    handle(err)
}

if err != nil {
    ...
}
```

This is legal.

A shadowing analyzer SHOULD consider warning because programmer may confuse inner and outer `err`.

#### 8.2.4 Typical Benign Example

```anuy
var user = rawUser()

if normalize {
    var user = normalizeUser(user)
    save(user)
}
```

A linter MAY choose not to warn if the pattern is clearly local and intentional.

Exact heuristics are non-normative.

#### 8.2.5 Lint: Shadowing

Anuy tooling SHOULD provide a shadowing analyzer.

Its purpose is:

> identify likely accidental shadowing without making ordinary lexical shadowing illegal.

#### 8.2.6 Recommended High-value Shadowing Warnings

Analyzer SHOULD strongly consider warning when:

1. local shadows package-level binding;
2. local shadows imported package name;
3. inner binding shadows `err`;
4. outer binding is used immediately after shadowing scope;
5. shadowed variable participates in return/error control flow;
6. shadowing may make assignment appear to affect outer state.

Exact heuristics are tooling policy.

#### 8.2.7 Lint False Positives

Because shadowing has legitimate uses, analyzer MUST be designed as advisory.

Examples such as:

```anuy
var value = raw()

if normalize {
    var value = normalizeValue(value)
}
```

may be intentionally accepted without warning.

### 8.3 Formatter

#### 8.3.1 Formatter

Formatter MUST preserve:

```text
var → declaration
=   → assignment
```

It never rewrites one into the other.

#### 8.3.2 Formatter and Shadowing

Formatter does not rename shadowed variables.

Shadowing policy belongs to compiler semantics and lint/refactoring tooling.

### 8.4 Diagnostics Catalog

Identifiers D-1…D-6 are local references inside the RFC; message forms are illustrative; stable `ANUY####` codes follow RFC-011 policy.

#### 8.4.1 D-1 — Unknown Assignment

```anuy
x = 10
```

without any visible `x`:

```text
unknown variable "x"

use "var x = 10" to declare it
```

#### 8.4.2 D-2 — Same-scope Redeclaration

```anuy
var x = 1
var x = 2
```

Recommended:

```text
"x" is already declared in this scope
```

#### 8.4.3 D-3 — No Error for Ordinary Shadowing

This is valid:

```anuy
var x = 1

{
    var x = 2
}
```

Compiler MUST NOT report a language error solely because outer `x` exists.

Lint MAY report a warning.

#### 8.4.4 D-4 — Mixed Multiple Declaration

```anuy
var x = 1
var x, y = operation()
```

Recommended:

```text
cannot declare "x": it is already declared in this scope

multiple declaration requires every name to be new in the current scope
```

#### 8.4.5 D-5 — Multiple Assignment Missing Binding

```anuy
var x int
x, y = operation()
```

Recommended:

```text
unknown variable "y"

multiple assignment requires every target to resolve to an existing binding
```

#### 8.4.6 D-6 — Uninitialized Outer Despite Shadow

```anuy
var x int

if condition {
    var x = 1
}

print(x)
```

Recommended:

```text
variable "x" may be uninitialized here

the assignment in the branch initializes a different shadowing binding
```

This is an important Anuy-specific diagnostic.

## 9. Rationale

### 9.1 Why Shadowing Is Allowed

Shadowing can express useful local transformations without inventing artificial names.

Example:

```anuy
var value = decode(input)

if normalize {
    var value = normalize(value)
    use(value)
}
```

Forcing:

```anuy
var normalizedValue = ...
```

is sometimes useful, but should be a style choice rather than a language requirement.

### 9.2 Why Shadowing Is Safer Than in Go Short Declarations

Anuy does not support:

```text
:=
```

Therefore programmer cannot accidentally create a binding through syntax that also performs reassignment.

In Anuy:

```anuy
var err = ...
```

visibly declares.

```anuy
err = ...
```

visibly assigns.

The main ambiguity associated with Go short declaration is absent.

## 10. Rejected Alternatives

### 10.1 Keep Go `:=`

Rejected.

Reasons:

- declaration and assignment can coexist in one statement;
- semantics depend on existing names;
- refactoring can alter declaration behavior;
- accidental shadowing/reuse is harder to see.

### 10.2 `:=` Only When All Names New

Rejected.

It would merely duplicate:

```anuy
var x, y = ...
```

with no semantic benefit.

### 10.3 Ban All Shadowing

Previously considered.

Example rejected rule:

```anuy
var x = 1

{
    var x = 2 // error
}
```

Not selected.

Reasons:

- `var` already makes declaration explicit;
- forbidding normal lexical shadowing creates naming friction;
- callback and loop parameters become awkward;
- Go migration requires unnecessary renaming;
- debugger/compiler already need lexical scope support;
- safety benefit is much smaller once `:=` is removed.

### 10.4 Explicit `shadow`

Example:

```anuy
shadow var x = ...
```

Rejected.

Reasons:

- unfamiliar for Go developers;
- repeats intent already expressed by `var`;
- adds syntax for a standard lexical-scope operation.

### 10.5 Restricted Shadowing Categories

For example:

```text
local → local        allowed
local → parameter    forbidden
local → package      forbidden
```

Rejected as core model.

This creates a policy table programmer must memorize.

Uniform lexical shadowing plus lint is simpler.

### 10.6 Rust-style Same-scope Rebinding

For example:

```anuy
var value = raw()
var value = normalized(value)
```

Rejected.

Anuy distinguishes same-scope binding identity clearly.

Same-scope redeclaration remains illegal.

Transformational shadowing requires a nested lexical scope.

### 10.7 Automatic Branch Binding Merge

Rejected.

Example:

```anuy
if condition {
    var x = 1
} else {
    var x = 2
}

print(x)
```

does not create a merged `x`.

Programmer should declare outer `x` explicitly.

### 10.8 Shadowing as Compile Error but Lint Suppression

Rejected.

If shadowing is fundamentally legal lexical behavior, requiring suppression annotations merely recreates explicit-shadow syntax indirectly.

## 12. Open Questions

Before `Accepted`:

1. exact loop syntax — resolved by owner decision 2026-09-14: Section 6.6.1 fixes the
   grammar (three `for` forms plus bare `break`/`continue`);
   range/index forms and labeled transfer remain assigned to Language Core;
2. whether `++` / `--` remain;
3. exact typed multiple declaration syntax;
4. blank identifier policy;
5. whether parameters remain mutable;
6. named result support;
7. exact import/value namespace interaction;
8. default severity of shadowing lint;
9. recommended suspicious-shadowing heuristics;
10. narrowing rules for captured/escaped locals — resolved: baseline —
    narrowing per binding identity (§13.25–27), §6.5.4/§6.5.7;
    conservative defaults of the future function model — Language Core
    (owner decision, 2026-09-16);
11. safe-navigation assignment — resolved: safe navigation is not an
    assignment target in v1 (§13.33); a safe-call statement is allowed
    (RFC-002 §6.4.3/§6.4.12, owner decision, 2026-09-17);
12. whether project configuration may promote shadow lint to error.

These questions do not block the Proposed status, but they must be resolved or retained with an explicitly assigned owner before Accepted.

- loop syntax, typed multiple declaration spelling, blank identifier policy, mutable parameters, named results and import/value namespaces → Language Core/reference manual;
- `++`/`--` retention → future Language Core decision;
- shadowing lint severity, heuristics and configuration → RFC-011;
- captured/escaped-local narrowing — resolved: baseline in both RFCs (§6.5.4/§6.5.7, RFC-002 §6.3.9); nuances — Language Core;
- safe-navigation assignment — resolved: §13.33.

## 13. Normative Summary

Normatively:

1. Anuy has no `:=`.
2. `var` always declares a new binding in the current lexical scope.
3. `=` never declares a binding.
4. Assignment resolves the nearest visible existing binding.
5. `var x = expr` declares with inferred type.
6. `var x T = expr` declares with explicit type.
7. `var x T` declares an uninitialized binding.
8. First initialization uses ordinary `=`.
9. Multiple declaration declares all listed names.
10. Every name in multiple declaration must be new in the current scope.
11. Multiple assignment requires all targets to already resolve.
12. Mixed declaration/reassignment is forbidden.
13. Same-scope redeclaration is forbidden.
14. Ordinary lexical shadowing of outer bindings is allowed.
15. No special `shadow` syntax exists.
16. A declaration is not in scope within its own initializer.
17. Therefore a shadowing initializer may reference the outer binding.
18. Parameters may be shadowed in nested scopes.
19. Closure parameters may shadow outer names.
20. Loop iteration bindings may shadow outer names.
21. Package bindings may be shadowed by locals.
22. Potentially accidental shadowing is a lint concern rather than a language error.
23. Lint MUST NOT redefine core language validity.
24. Branch-local declarations do not merge by name.
25. Definite initialization is tracked per binding identity.
26. Nullability refinement is tracked per binding identity.
27. Shadowing binding does not initialize or mutate outer binding.
28. Loops that may execute zero times cannot establish post-loop initialization by body assignment alone.
29. Every reachable loop exit participates in initialization proof.
30. Closure reads follow RFC-001 initialization rules.
31. Arbitrary closure/function calls do not establish caller-local initialization.
32. Iteration variables SHOULD have per-iteration logical identity.
33. Safe navigation is not an assignment target in v1.
34. Address-taking uninitialized binding is forbidden by baseline rules.
35. Compiler-generated temporaries are synthetic.
36. Tooling MUST distinguish shadowed bindings by semantic identity.

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (design filter);
- RFC-001 — Values, Initialization and Trust (definite initialization, closure capture);
- RFC-002 — Nullability and Nil Safety (flow refinement per binding identity);
- RFC-004 — Interfaces and Explicit `impl` (method receivers/parameters);
- RFC-005 — Error Handling and Propagation (`try` failure path, `discard`);
- RFC-006 — Enums and Exhaustive Matching (`match` initialization);
- RFC-007 — Unsafe and Foreign Contracts (unsafe does not change scope rules);
- RFC-008 — Go Interoperability (foreign callbacks, out parameters);
- RFC-009 — Lowering and Generated Go Contract (binding identity in lowering);
- RFC-011 — Diagnostics, Debugging and Tooling (lint, codes, rename, debugger).

### 14.2 Informative
