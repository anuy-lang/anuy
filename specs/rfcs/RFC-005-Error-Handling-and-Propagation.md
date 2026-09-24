# RFC-005 — Error Handling and Propagation

**Status:** Accepted
**RFC:** 005
**Title:** Error Handling and Propagation
**Language:** Anuy
**Area:** Errors / Control Flow / Type System / Go Interoperability
**Version:** 8
**Date:** 2026-09-25
**Requires:** RFC-000, RFC-001, RFC-002, RFC-003, RFC-004, RFC-007, RFC-008, RFC-009, RFC-011
**Supersedes:** —

---

## Decision Record

RFC-005 was reviewed against RFC-001 (conditional initialization of success results), RFC-002 (`error?` as an ordinary nullable type) and RFC-003 (multi-value declaration/assignment). Delegations: trust of foreign error values — RFC-007 (Section 7.6); strict/partial classification of foreign results and typed-nil normalization — RFC-008 (Section 7.7); synthetic naming, ABI padding, SourceMap encoding — RFC-009 (Section 7.8).

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

---

Version 3, 2026-09-20: OQ-3 (Section 12) added — the failure-return surface form for strict fallible functions (`return error err` / `return _, err` / type-driven `return err`) is moved to an explicit decision; the error-only canon (Section 6.4.4, `return err`) is unchanged (owner question, 2026-09-20).

Version 4, 2026-09-21: strict-slice preparation — §6.4.5 receives diagnostic D-7 (invalid mixed return, §8.1.7); §6.4.7 added (fall-off from a fallible function: strict — D-6 missing return, error-only — implicit success `nil`); OQ-3 resolved by the owner — variant 1 `return error err` (owner decision, 2026-09-21, accepted); §13 extended with items 41–42 (design, 2026-09-21).

Version 5, 2026-09-21: correlation-slice preparation — §6.3.1/§6.3.3 receive diagnostic pins D-4; §6.3.7 added (correlation and loops: materialization does not escape the body, the conservative rule of mature compilers); §13 extended with items 43–44 (design, 2026-09-21).

Version 6, 2026-09-21: OQ-4 (Section 12) added — early-exit conditional binding in the spirit of Swift guard / Rust let-else (`var data, err = Load() else { … }`), proposed by the owner; the surface form and its adoption are an owner question; implementation is a separate grammar slice over correlation (design, 2026-09-21).

Version 7, 2026-09-21: must-consume slice preparation — §6.6.5 receives the diagnostic pin D-1 (blank error target in destructuring — bypassing must-consume); §8.1.1 clarified: D-1 covers surface call statements and the blank error target, while R1/ANUY5001 remains the binding-surface lint for `error?` bindings read nowhere (design, 2026-09-21).

Version 8, 2026-09-25: published as the canonical English text of the Accepted RFC; no semantic changes.

## 1. Abstract

Anuy keeps the Go error model — ordinary error values, explicit returns, the ordinary ABI — and introduces no exceptions, no `Result<T, E>`, no `throws`, and no implicit error swallowing. A fallible function has a trailing `error?`; a failure return is written `return error err`; propagation is `try`, which evaluates the operation exactly once and, on a non-null error, immediately returns it from the current function. Success results are conditionally initialized until `err == nil` is proven, the error result cannot be silently ignored (only an explicit `discard`), and failure lowering uses synthetic Go ABI padding invisible to safe Anuy code.

## 2. Solutions

> **Errors are values, failure is explicit, propagation is visible, and successful values do not magically exist on failure paths.**

> **Go errors. Checked handling. Explicit propagation. No hidden zero values.**

> **Errors remain values.**

> **Fallibility is visible in the type.**

> **Errors cannot disappear accidentally.**

> **Propagation is visible control flow.**

> **Native Anuy fallible functions are strict by default.**

## 3. Motivation

In Go an error is an ordinary value, but the language does nothing to keep you from losing it: `err` can be forgotten, and the `if err != nil` boilerplate repeats in every function. Existing languages solve this with exceptions (hidden control flow, alien to the Go model) or `Result`/`?` (a new core type and ecosystem). Anuy takes the third path: keep Go error values and the ABI, make propagation an explicit control construct (`try`), make losing an error a compile error (`discard` for intentional ignoring), and tie all of it to definite initialization and nullability analysis.

## 4. Goals

The following goals follow from the design principles and the decisions of this RFC: the error model stays Go-compatible; fallible signatures are checkable; propagation is visible; success values are not observable on the failure path; interop with the Go error ecosystem (`errors.Is/As`, `%w`, `errors.Join`) is preserved; failure lowering introduces no new runtime mechanism.

## 5. Non-goals

This RFC does not define:

- exceptions and panic/recover semantics;
- a `Result<T, E>` core type;
- automatic error wrapping/logging;
- a cleanup-error policy (`defer try`);
- foreign contract syntax and the classification of partial APIs (RFC-008);
- goroutine/concurrency syntax details.

## 6. Specification

### 6.1 Design Principles

#### 6.1.1 Errors remain values

An error is an ordinary value.

It can be:

- stored;
- passed to a function;
- wrapped;
- compared via standard mechanisms;
- returned;
- placed in an aggregate;
- logged.

There is no hidden exception channel.

#### 6.1.2 Fallibility is visible in the type

```anuy
func Save(data Data) error?
```

explicitly says the operation can fail.

```anuy
func Hash(data Data) Hash
```

has no error result.

#### 6.1.3 Errors cannot disappear accidentally

A call returning an error result cannot be silently ignored.

The programmer chooses one of:

```text
handle
propagate
return
explicitly discard
```

#### 6.1.4 Propagation is visible control flow

`try` contains an early return.

That is why RFC-005 deliberately does not turn `try` into an unrestricted operator that can be hidden inside an arbitrary expression tree.

### 6.2 Error Type and Fallible Pipeline

#### 6.2.1 The `error` type

Anuy uses the standard Go-compatible error interface.

Conceptually:

```anuy
interface error {
    Error() string
}
```

Per RFC-004, a native Anuy type intended to serve as an error must declare conformance explicitly:

```anuy
type ParseError {
    ...
}

func (err *ParseError) Error() string {
    ...
}

impl error for *ParseError
```

`error` is a non-null type.

```anuy
var err error
```

means an uninitialized non-null error binding.

`error?` is a nullable error:

```anuy
var err error? = nil
```

#### 6.2.2 Why function errors use `error?`

A successful operation has no error value.

Therefore an ordinary fallible function uses:

```text
error?
```

not:

```text
error
```

Example:

```anuy
func Save(data Data) error?
```

Semantic outcomes:

```text
nil
    success

non-null error
    failure
```

This is a direct application of RFC-002:

```text
Values(error?) = Values(error) ∪ {nil}
```

#### 6.2.3 Fallible signatures

A native function is a **fallible function** if its last result has type:

```text
error?
```

after alias expansion.

Examples:

```anuy
func Save(data Data) error?

func Load(path string) (Data, error?)

func Parse(input string) (Node, Diagnostics, error?)
```

The last result is called the **error result**.

All preceding results are called **success results**.

#### 6.2.4 Error values outside the final result

`error?` remains an ordinary type and can appear elsewhere:

```anuy
struct Attempt {
    error error?
}
```

A function can also technically have an unusual result shape:

```anuy
func Strange() (error?, int)
```

But such a signature is not a fallible signature of RFC-005.

`try` applies only to a trailing `error?`.

Native APIs SHOULD use the conventional trailing error result.

#### 6.2.5 Native fallible result semantics

For a native Anuy function:

```anuy
func Load() (Data, Metadata, error?)
```

the semantic result conceptually has two forms:

```text
Success(Data, Metadata)

Failure(error)
```

This is **not** a user-visible sum type.

The language has no:

```text
Result<Data, error>
```

It is the semantic model of the compiler IR.

On the success path:

```text
Data       initialized
Metadata   initialized
error      nil
```

On the failure path:

```text
error      non-null
success results have no observable Anuy value
```

#### 6.2.6 Semantic model

The full native fallible pipeline:

```text
call
 ↓
FallibleResult
 ├── Success(values...)
 │      ↓
 │   values initialized
 │
 └── Failure(error)
        ↓
     success values unavailable
```

`try`:

```text
FallibleResult
 ├── Success(values...)
 │      ↓
 │   continue
 │
 └── Failure(error)
        ↓
     early return failure
```

### 6.3 Conditional Initialization and Correlation

#### 6.3.1 Why success results are conditional

Consider:

```anuy
var data, err = Load()
```

If `err != nil`, the compiler must not invent an implicit Anuy default value for `data`.

Therefore `data` gets the state:

```text
initialized iff err == nil
```

This is **conditional initialization**.

Example:

```anuy
var data, err = Load()

print(data)
```

is a compile error.

The compiler has not yet proven that `data` exists as a semantic value.

Diagnostic — Section 8.1 (D-4).

#### 6.3.2 Error refinement initializes success results

Correct:

```anuy
var data, err = Load()

if err != nil {
    return error err
}

print(data)
```

After:

```anuy
if err != nil {
    return ...
}
```

the compiler knows:

```text
err == nil
```

on the remaining path.

Consequently:

```text
data initialized
```

This combines RFC-001 definite initialization with RFC-002 control-flow refinement.

#### 6.3.3 Branch-local refinement

Also:

```anuy
var data, err = Load()

if err == nil {
    use(data)
}
```

is valid inside the success branch.

But:

```anuy
var data, err = Load()

if err != nil {
    log(err)
}

use(data)
```

is an error.

The non-null branch can fall through, so the compiler has not proven the successful result.

Diagnostic — Section 8.1 (D-4).

#### 6.3.4 Replacing a missing success result

The programmer may initialize the value themselves on the failure path:

```anuy
var config, err = LoadConfig()

if err != nil {
    config = defaultConfig()
}

use(config)
```

After the join:

```text
config initialized on every path
```

so the code is valid.

#### 6.3.5 Correlation identity

Conditional initialization is tied not just to the name `err` but to the specific result group.

The semantic IR SHOULD have an identity like:

```text
FallibleResultID
```

For example:

```text
load#42:
    value#17 initialized when err#18 == nil
```

Shadowing does not break correctness, because the analysis works through `SymbolID`.

#### 6.3.6 Reassignment kills correlation

If the controlling error binding is reassigned:

```anuy
var data, err = Load()

err = anotherError()

if err == nil {
    use(data)
}
```

that is not enough.

The link:

```text
data initialized iff original err == nil
```

has been lost.

The compiler MUST conservatively invalidate the corresponding refinement when the controlling binding is reassigned.

#### 6.3.7 Correlation and loops

Conditional initialization enters the loop body as conditional state —
without proofs.

Materialization proven inside the body does not escape the loop: after
the loop a new proof is required.

```anuy
var data, err = Load()

for cond {
    use(data)
}

use(data)
```

both `use`s are a compile error (D-4).

An assignment to the controlling error inside the body kills the
correlation for the rest of the body (Section 6.3.6).

This conservative rule matches definite assignment in mature compilers
(Java, Go): loop-carried state does not inherit proofs from the last
iteration, and the zero-iteration path carries the pre-loop state.

### 6.4 Returns

#### 6.4.1 Success return

An ordinary successful return keeps Go-like syntax.

```anuy
func Load() (Data, error?) {
    var data = ...
    return data, nil
}
```

For several success values:

```anuy
return node, diagnostics, nil
```

The compiler checks that all returned success expressions are fully initialized.

#### 6.4.2 Failure return

For a function that has success results, Anuy introduces an explicit failure return:

```anuy
return error err
```

Example:

```anuy
func Load(path string) (Data, error?) {
    if path == "" {
        return error errors.New("empty path")
    }

    ...
}
```

`error` after `return` is a contextual keyword.

The operand MUST have a non-null type:

```text
error
```

or a type implicitly assignable to `error`.

Diagnostic — Section 8.1 (D-5).

`error` is a contextual keyword only in return position; outside it, it is an ordinary
identifier (Section 6.2.4 allows a field named `error`; local `var error ...` is valid).
Consequently, a binding named `error` cannot be returned in bare form — `return error`
parses as the start of the failure form; tooling diagnostics suggest a rename (the Go
convention is `err`) or the explicit form `return error error` with a proven non-null
operand.

#### 6.4.3 Why `return error`

Go requires:

```go
return Data{}, err
```

but Anuy has no implicit/default `Data{}` semantics for an arbitrary type.

`return error err` expresses the actual programmer intent:

> the function failed; the success result does not exist.

The compiler creates the necessary ABI padding values during lowering.

They are not Anuy values.

#### 6.4.4 Error-only functions

If the function returns only:

```anuy
func Save() error?
```

the ordinary syntax remains sufficient:

```anuy
return nil
```

for success and:

```anuy
return err
```

for failure.

`return error err` MAY also be accepted for consistency, but the canonical style for an
error-only function is:

```anuy
return err
```

#### 6.4.5 Invalid mixed native return

For a strict native fallible function:

```anuy
func F() (T, error?)
```

the form:

```anuy
return value, err
```

where `err` may be non-null, is forbidden.

Two semantic forms are allowed:

```anuy
return value, nil
```

or:

```anuy
return error err
```

This supports the strong invariant:

```text
success values exist iff error == nil
```

Diagnostic — Section 8.1 (D-7).

#### 6.4.6 Partial-result APIs

Some Go APIs have a different contract.

For example, an operation may return simultaneously:

```text
useful partial value
+
non-nil error
```

This matters for APIs like stream I/O.

Such semantics is called in this RFC:

```text
partial / raw fallible result
```

It is **not** the default native Anuy fallible semantics.

The exact classification of imported Go APIs and foreign contract methods is defined by RFC-008.

RFC-005 fixes:

> Native Anuy fallible functions are strict by default.

Foreign contracts MAY explicitly opt into partial-result semantics.

#### 6.4.7 Fall-off from a fallible function

An error-only fallible function:

```anuy
func Save() error?
```

with control reaching the end of the body without `return` means success:

```text
implicit return nil
```

This continues the nullable-result semantics of RFC-001: semantic `nil` is
a valid value of a nullable result.

A strict fallible function (one with at least one success result):

```anuy
func Load() (Data, error?) {
    var data = ...
}
```

may not fall off the end — a compile error.

Neither success (`return data, nil`) nor failure (`return error err`) is
implied: the success results have no value to substitute implicitly, and a
silent success with an unset value would violate definite initialization
(RFC-001).

Diagnostic — D-6 missing return (RFC-001 §13.18); the same policy as for
declared non-null results (owner decision, 2026-09-20): all mature
ecosystems report it — Go "missing return", C# CS0161, Rust, Kotlin.

### 6.5 `try`

#### 6.5.1 `try`

The primary propagation form:

```anuy
try expression
```

For:

```anuy
var file = try Open(path)
```

where:

```anuy
Open(path) : (*File, error?)
```

the result of `try Open(path)` has the success type:

```text
*File
```

The trailing `error?` is stripped from the produced success values.

#### 6.5.2 Multiple success values

If:

```anuy
operation() : (A, B, error?)
```

then:

```anuy
var a, b = try operation()
```

is valid.

`try operation()` produces:

```text
(A, B)
```

on the success path.

#### 6.5.3 Error-only `try`

If:

```anuy
Save() : error?
```

then:

```anuy
try Save()
```

is allowed as a statement.

The success result pack is empty.

Conceptually:

```text
call Save

if err != nil:
    propagate err

continue
```

#### 6.5.4 `try` requires a fallible enclosing function

`try` may early-return an error.

Therefore the enclosing function must itself have a trailing `error?`.

For example:

```anuy
func Load() (Data, error?) {
    var file = try Open()
    ...
}
```

is valid.

But:

```anuy
func Load() Data {
    var file = try Open()
}
```

is a compile error.

Diagnostic — Section 8.1 (D-6).

#### 6.5.5 `try` is not a general unary operator

Rejected:

```anuy
process(try Load())
```

Rejected:

```anuy
var size = 10 + try ReadSize()
```

Rejected:

```anuy
if try Ready() {
    ...
}
```

Rejected:

```anuy
var value = Container{
    item: try Load(),
}
```

Even if the type system could theoretically lower it.

The reason:

> the early return must remain visually obvious.

#### 6.5.6 Allowed `try` positions

In v1 `try` is allowed in the following propagation positions.

**Declaration RHS:**

```anuy
var file = try Open(path)
```

```anuy
var data, meta = try Load(path)
```

**Assignment RHS:**

```anuy
file = try Reopen(path)
```

```anuy
x, y = try Compute()
```

**Standalone statement:**

```anuy
try Save(data)
```

`try` MUST occupy the entire semantic RHS.

#### 6.5.7 No nested propagation

Not allowed:

```anuy
var x = transform(try load())
```

Instead:

```anuy
var value = try load()
var x = transform(value)
```

This is slightly more verbose, but the control flow remains obvious.

This matches the Anuy principle:

> Explicit where correctness depends on intent.

#### 6.5.8 Evaluation exactly once

The operand of `try` is evaluated exactly once.

```anuy
var data = try source.next()
```

cannot be lowered in a way that re-evaluates:

```text
source
```

or the arguments.

Side effects happen once.

#### 6.5.9 Propagated error is unchanged

`try` itself does not:

- wrap;
- annotate;
- convert to string;
- join;
- log;
- change identity.

If the callee returns `err`, the caller propagates the same error value.

Conceptually:

```text
return error err
```

#### 6.5.10 Adding context

If the programmer wants to add context, that is explicit code.

For example:

```anuy
var value, err = Load(path)

if err != nil {
    return error fmt.Errorf("load config: %w", err)
}

use(value)
```

Anuy reuses the Go error ecosystem:

- `errors.Is`;
- `errors.As`;
- `%w`;
- `errors.Join`;
- custom error types.

No language-level wrapping mechanism is required.

#### 6.5.11 Explicit handling

The programmer is not required to use `try`.

The error can be handled directly:

```anuy
var file, err = Open(path)

if err != nil {
    log(err)
    return error err
}

use(file)
```

This is a first-class supported style.

`try` is a shorthand for the common propagation pattern, not a mandatory error model.

### 6.6 Must-Consume and Discard

#### 6.6.1 Error-result must be consumed

A fallible result cannot be silently dropped.

This is an error:

```anuy
file.Close()
```

if:

```text
Close() : error?
```

This is also an error:

```anuy
Load(path)
```

if the return contains a trailing `error?`.

The programmer must handle the result explicitly.

Diagnostic — Section 8.1 (D-1).

#### 6.6.2 `discard`

For intentional ignoring there is:

```anuy
discard expression
```

For example:

```anuy
discard file.Close()
```

It means:

> evaluate this operation and intentionally discard all its results, including its error result.

`discard` is an explicit opt-out.

#### 6.6.3 Why not silent expression statements

Go allows:

```go
file.Close()
```

and silently discards the returned error.

Anuy deliberately rejects that semantics.

The difference between:

```anuy
file.Close()
```

and:

```anuy
discard file.Close()
```

matters: the second form carries the programmer's intent.

#### 6.6.4 `discard` does not mean success

```anuy
discard operation()
```

does not mean:

```text
assert operation succeeds
```

and does not mean:

```text
panic on error
```

It is only an explicit ignore.

Tooling MAY lint suspicious discarded errors.

For example:

```text
discard transaction.Commit()
```

may deserve a warning.

But language validity is already settled by `discard`.

#### 6.6.5 Blank identifier does not bypass error checking

If Anuy supports `_` for other multi-value operations, the error result cannot be silently dropped:

```anuy
var value, _ = Load()
```

is not a valid way to ignore the error.

For the error result, the intent must be expressed through an error handling construct.

Diagnostic — Section 8.1 (D-1).

This prevents bypassing:

```text
must-handle error
```

via the blank identifier.

#### 6.6.6 Passing errors onward

An error is considered consumed if the programmer explicitly passes it on:

```anuy
var value, err = Load()

if err != nil {
    report(err)
    return error err
}
```

The compiler does not try to prove that `report` actually "handled" the error.

The guarantee of RFC-005:

> the error result cannot disappear accidentally.

Not:

> the compiler can prove every application-level error policy is correct.

### 6.7 Forwarding and Contexts

#### 6.7.1 Direct tail forwarding

If the current function has a compatible fallible signature, the following is allowed:

```anuy
func Load() (Data, error?) {
    return ReadData()
}
```

This is not an error ignore.

It is explicit forwarding of the entire fallible result.

There is no intermediate observation of the success results.

The compiler checks the compatibility of the full result protocol.

#### 6.7.2 `return try` is unnecessary

Not introduced:

```anuy
return try ReadData()
```

Use:

```anuy
return ReadData()
```

if forwarding is intended.

And:

```anuy
var data = try ReadData()
...
return data, nil
```

if the caller continues after the successful result.

#### 6.7.3 `try` in closures

`try` returns from the nearest enclosing function/closure.

For example:

```anuy
var callback = func() error? {
    try Save()
    return nil
}
```

is valid.

But `try` inside a callback does not propagate from the function that created the callback.

Every function boundary is independent.

#### 6.7.4 `try` in loops

```anuy
for item in items {
    try Save(item)
}
```

if the standalone `try` statement grammar is allowed in a loop body, propagates from the enclosing function.

It does not mean:

```text
continue
```

or:

```text
break
```

#### 6.7.5 Goroutines

An error must also not silently disappear through a goroutine invocation.

If:

```anuy
func Work() error?
```

then the direct form:

```anuy
go Work()
```

SHOULD be rejected.

The programmer must define the policy inside the goroutine.

For example, conceptually:

```anuy
go func() {
    var err = Work()

    if err != nil {
        report(err)
    }
}()
```

The exact goroutine syntax remains Go-compatible and may be refined by a separate concurrency RFC/tooling spec.

#### 6.7.6 Panic is not error propagation

`try` does not:

- catch panics;
- translate panics to errors;
- invoke `recover`;
- change panic semantics.

```text
error
```

and:

```text
panic
```

remain different mechanisms.

RFC-005 deals only with error values.

#### 6.7.7 Safe navigation and errors

Safe navigation and propagation are different constructs.

Complex combinations like:

```anuy
try receiver?.load()
```

are not part of the v1 automatic composition.

If a nullable receiver decides whether the fallible operation runs, the programmer must make the control flow explicit.

For example:

```anuy
if receiver != nil {
    var value = try receiver.load()
    ...
}
```

This avoids ambiguity between:

```text
receiver absent
```

and:

```text
operation failed
```

### 6.8 Defer and Cleanup

#### 6.8.1 `defer`

Fallible deferred calls require special attention.

The ordinary:

```anuy
defer file.Close()
```

is forbidden if `Close()` returns `error?`.

The reason:

the error would disappear implicitly.

#### 6.8.2 Explicitly ignored deferred error

Allowed:

```anuy
defer discard file.Close()
```

Semantics:

1. the receiver and arguments are evaluated at the `defer` point, following Go-compatible defer rules;
2. the call runs at function exit;
3. the result is explicitly discarded.

The intent is visible.

#### 6.8.3 No `defer try` in v1

RFC-005 v1 **does not introduce**:

```anuy
defer try file.Close()
```

The reason is the conflict of several errors.

For example, the body already propagates:

```text
write error
```

while the deferred `Close()` simultaneously returns:

```text
close error
```

The language would have to implicitly choose:

- the first error;
- the last error;
- a joined error;
- a wrapped error;
- a replacement error.

None of the options is universally right.

Therefore:

> Cleanup errors that matter require explicit programmer policy.

#### 6.8.4 Handling cleanup errors explicitly

The programmer can organize the control flow explicitly.

For example:

```anuy
var file = try Open(path)

var writeErr = WriteAll(file, data)

var closeErr = file.Close()

if writeErr != nil {
    if closeErr != nil {
        return error errors.Join(writeErr, closeErr)
    }

    return error writeErr
}

if closeErr != nil {
    return error closeErr
}

return nil
```

Library helpers MAY encapsulate common policies.

But the core language does not choose a policy automatically.

### 6.9 Dataflow and Foreign Contracts

#### 6.9.1 Native strict fallible calls and dataflow

For:

```anuy
var value, err = operation()
```

where `operation` has a native strict fallible signature:

```text
(T, error?)
```

the compiler records:

```text
err initialized

value:
    conditionally initialized
    guard = (err == nil)
```

At:

```anuy
if err == nil {
```

the success values become initialized in the branch.

At:

```anuy
if err != nil {
    return ...
}
```

they are initialized after the branch.

#### 6.9.2 `try` and initialization

`try` naturally eliminates the conditional state.

```anuy
var value = try operation()
```

after the declaration:

```text
value definitely initialized
```

because execution reaches the next statement only in the success case.

This is one of the main reasons to integrate error handling with the typed semantic IR.

#### 6.9.3 Foreign Go calls

Imported Go APIs usually have signatures like:

```go
func Open(name string) (*File, error)
```

The Anuy-facing signature is conceptually:

```anuy
func Open(name string) (*File, error?)
```

so:

```anuy
var file = try os.Open(path)
```

is naturally supported.

The exact trust contracts of imported Go results belong to RFC-007/RFC-008.

#### 6.9.4 Strict vs partial foreign contracts

RFC-008 must be able to distinguish at minimum:

```text
strict fallible result
partial/raw fallible result
```

Strict:

```text
error != nil
→
success results semantically unavailable
```

Partial:

```text
error != nil
→
some preceding values MAY remain meaningful
```

For a raw/partial call, explicit handling may read both:

```anuy
var n, err = reader.Read(buffer)

if n > 0 {
    consume(buffer[:n])
}

if err != nil {
    ...
}
```

This is required for faithful Go interoperability.

#### 6.9.5 `try` on partial operations

`try` MAY semantically mean:

```text
if error != nil:
    discard partial values
    propagate error
```

But tooling SHOULD be able to warn when `try` is used on a foreign API whose contract marks partial values as significant.

RFC-008 may make some such operations non-tryable without explicit acknowledgment.

RFC-005 does not fix foreign metadata syntax.

### 6.10 Custom Errors and Typed Nil

#### 6.10.1 Error method implementations

A custom native error:

```anuy
type ParseError {
    offset int
    message string
}

func (err *ParseError) Error() string {
    ...
}

impl error for *ParseError
```

can be used as:

```anuy
return error &ParseError{
    offset: offset,
    message: "unexpected token",
}
```

The exact aggregate initialization syntax is defined by the corresponding type RFC.

#### 6.10.2 Typed nil

Safe native Anuy must not create the confusing Go-style state:

```text
non-nil error interface
containing nil *ConcreteError
```

when the underlying pointer source type is non-null.

Foreign Go may pass such a value across the boundary.

That is a foreign trust issue of RFC-007/RFC-008.

RFC-005 does not attempt to change the Go runtime interface representation.

### 6.11 Lowering and ABI

#### 6.11.1 Lowering `try`

Source:

```anuy
func Load(path string) (Data, error?) {
    var file = try os.Open(path)
    return parse(file), nil
}
```

Conceptual generated Go:

```go
func Load(path string) (Data, error) {
    file, __anuy_err1 := os.Open(path)

    if __anuy_err1 != nil {
        var __anuy_padding0 Data
        return __anuy_padding0, __anuy_err1
    }

    return parse(file), nil
}
```

`__anuy_padding0` exists only for the Go ABI.

#### 6.11.2 ABI padding is not an Anuy default value

The synthetic:

```go
var __anuy_padding0 Data
```

does not mean that Anuy introduced:

```text
Default<Data>
```

or implicit zero initialization.

The source-semantics failure result contains only:

```text
error
```

The padding is:

- synthetic;
- backend-only;
- not a semantic source value;
- must not be observable by safe Anuy code.

#### 6.11.3 External Go consumers

Generated Go functions keep the ordinary Go ABI:

```go
func Load(...) (Data, error)
```

For native strict Anuy functions, the public contract for Go consumers is:

> When `error != nil`, preceding result values are unspecified failure padding and must not be relied upon.

This is compatible with the ordinary Go error convention.

APIs that need meaningful partial results must have an explicit partial/raw contract.

#### 6.11.4 Multiple return lowering

Source:

```anuy
func Parse(path string) (Node, Metadata, error?) {
    var file = try Open(path)
    ...
}
```

Failure lowering, conceptually:

```go
var __anuy_padding0 Node
var __anuy_padding1 Metadata

return __anuy_padding0, __anuy_padding1, err
```

The number of padding slots is determined by the result signature.

#### 6.11.5 Error-only lowering

Source:

```anuy
func Save() error? {
    try flush()
    return nil
}
```

Conceptually:

```go
func Save() error {
    if err := flush(); err != nil {
        return err
    }

    return nil
}
```

No synthetic success results are needed.

### 6.12 Source Mapping, Debugging, Coverage and IDE

#### 6.12.1 SourceMap

Compiler-generated propagation control flow MUST have source mapping.

The recommended `SourceKind`:

```text
ErrorPropagation
```

The SourceMap must link the synthetic:

- temporary error binding;
- nil check;
- failure return;
- ABI padding;

to the source span of:

```anuy
try expression
```

#### 6.12.2 Diagnostics should point to `try`

If the generated Go or the compiler backend produces a diagnostic inside synthetic propagation, the user-facing location must point to:

```anuy
var value = try operation()
                ^^^^^^^^^^^^^
```

not to the generated:

```go
__anuy_err17
```

or:

```go
__anuy_padding3
```

#### 6.12.3 Debugging

The debugger SHOULD show the user-visible control flow as:

```text
call
→
error check
→
early return
```

Synthetic temporaries MAY be hidden by the DAP proxy/tooling.

Stack frames remain ordinary Go frames.

`try` creates no runtime frame.

#### 6.12.4 Coverage

The synthetic propagation branch is compiler-generated control flow.

Coverage reporting SHOULD map it to the source-level `try`.

Synthetic ABI padding must not create separate user-visible statements.

The compiler CoverageMap decides whether the error branch counts as a separate source branch metric.

#### 6.12.5 IDE support

The LSP SHOULD provide for `try`:

- hover with the original fallible signature;
- the success result type;
- the propagated error type;
- the enclosing function target;
- diagnostics if the enclosing function is non-fallible.

For example, hover:

```text
try os.Open(path)

callee:
    (*os.File, error?)

success:
    *os.File

on error:
    returns error from LoadConfig
```

## 7. Interaction with Other RFCs

### 7.1 RFC-001

RFC-001 states:

```text
uninitialized ≠ nil
```

RFC-005 continues this rule.

After:

```anuy
var data, err = Load()
```

with `err != nil`:

```text
data
```

does not become:

```text
zero Data
```

It is semantically unavailable.

This is conditional initialization, not nullability.

### 7.2 RFC-002

`error?` uses ordinary universal nullability.

There is no special "error nil".

```text
error
```

is non-null.

```text
error?
```

is nullable.

Flow narrowing:

```anuy
if err != nil {
    // err: error
}
```

works by the general RFC-002 rules.

### 7.3 RFC-003

Multi-value declaration:

```anuy
var data, err = Load()
```

creates both bindings.

Assignment:

```anuy
data, err = Load()
```

requires existing bindings.

`try` follows the same model:

```anuy
var data = try Load()
```

is a declaration.

```anuy
data = try Load()
```

is an assignment.

There is no `:=` error pattern.

### 7.4 RFC-004

Error interface conformance follows explicit `impl`.

A native custom error type does not become `error` accidentally just because it has:

```anuy
Error() string
```

It requires:

```anuy
impl error for *MyError
```

This makes the programmer's intent explicit for errors too.

### 7.5 RFC-007

RFC-007 must define the trust implications of foreign error values:

- typed nil;
- malformed foreign contracts;
- unsafe conversions;
- foreign values violating Anuy invariants.

RFC-005 introduces no separate trust model.

### 7.6 RFC-008

RFC-008 must define:

1. the mapping of the Go `error` result → Anuy `error?`;
2. the classification of strict vs partial foreign results;
3. Go interfaces with partial-result contracts;
4. foreign callbacks;
5. generated wrappers, if needed;
6. typed-nil normalization;
7. whether metadata can mark some APIs unsafe for automatic `try`.

### 7.7 RFC-009

RFC-009 must standardize:

- exact synthetic temporary naming;
- ABI failure padding;
- the emitted Go shape;
- optimization permissions;
- the exported strict-error contract;
- SourceMap encoding.

But RFC-009 cannot change the source semantics of this RFC.

## 8. Diagnostics and Tooling

### 8.1 Diagnostics Catalog

The identifiers D-1…D-6 are local references within this RFC; the message forms are illustrative; stable `ANUY####` codes follow the policy of RFC-011.

#### 8.1.1 D-1 — Ignored Error

```text
error: result of file.Close contains an error and cannot be ignored

    file.Close()
    ^^^^^^^^^^^^

handle the error, propagate it with `try`,
or explicitly ignore it:

    discard file.Close()
```

#### 8.1.2 D-2 — Invalid `try`

```text
error: try requires a trailing error? result

    var value = try compute()
                    ^^^^^^^^^

compute returns:
    int
```

#### 8.1.3 D-3 — Hidden `try`

```text
error: try cannot be nested inside another expression

    process(try Load())
            ^^^^^^^^^^

write:

    var value = try Load()
    process(value)
```

#### 8.1.4 D-4 — Unavailable Success Result

```text
error: data is not initialized on the error path

    var data, err = Load()
    use(data)
        ^^^^

data is available only when err == nil

check or propagate err before using data
```

#### 8.1.5 D-5 — Invalid Failure Return

```anuy
return error err
```

when `err : error?` and the compiler has not proven it non-null:

```text
error: failure return requires a non-null error

err has type:
    error?

narrow it first:

    if err != nil {
        return error err
    }
```

#### 8.1.6 D-6 — `try` in Non-fallible Function

```text
error: try may propagate an error,
but Load does not return error?
```

#### 8.1.7 D-7 — Invalid Mixed Return

```anuy
return value, err
```

for a strict fallible function (Section 6.4.5), where the trailing operand has
type `error?` and is not the literal `nil`:

```text
error: mixed return is neither a success nor a failure form

use:

    return value, nil

for success or:

    return error err

for failure
```

## 10. Rejected Alternatives

### 10.1 Language-level catch

RFC-005 does not introduce:

```anuy
try {
    ...
} catch {
    ...
}
```

Errors are handled with ordinary control flow:

```anuy
var value, err = operation()

if err != nil {
    ...
}
```

This preserves the Go-like execution model.

### 10.2 Core `Result<T, E>`

Rejected as a mandatory core model:

```text
Result<T, E>
Ok(T)
Err(E)
```

Reasons:

- the Go ABI already has multi-result + `error`;
- the Go ecosystem is built around `error`;
- wrappers would require pervasive conversion;
- interface compatibility would become harder;
- generated Go would stop being transparent.

Libraries MAY define their own sum/result types once the corresponding language support appears.

But core error propagation does not require them.

### 10.3 Exceptions

Rejected:

```anuy
throw err
```

and implicit stack unwinding.

Reasons:

- the execution model diverges from Go;
- the ABI/interop becomes more complicated;
- control flow becomes less locally visible;
- debugging and generated Go require additional machinery.

`try` remains a compile-time shorthand for an explicit return.

### 10.4 Postfix `?`

Rejected:

```anuy
var file = os.Open(path)?
```

Instead:

```anuy
var file = try os.Open(path)
```

Reasons:

- `?` is already the nullability syntax;
- postfix punctuation poorly shows the early return;
- `try` reads better as a control-flow operation.

### 10.5 Automatic wrapping

Rejected:

```text
try automatically adds the current function name
```

or the source location to the error chain.

Reasons:

- the error identity changes;
- predictable `errors.Is` breaks;
- hidden allocation/work is created;
- the programmer loses control over public error messages.

Context wrapping remains explicit.

### 10.6 Implicit logging

Propagation never automatically logs the error.

Otherwise one error could be logged at every stack level.

The policies:

```text
returning
logging
wrapping
retrying
discarding
```

remain independent programmer decisions.

### 10.7 Automatic cleanup-error policy

RFC-005 does not choose between:

```text
body error
cleanup error
errors.Join(body, cleanup)
```

The programmer or a library explicitly defines the policy.

Therefore `defer try` is absent in v1.

## 12. Open Questions

- **OQ-1 Goroutine syntax.** The exact goroutine syntax (Section 6.7.5) remains Go-compatible and may be refined by a separate concurrency RFC/tooling spec.
- **OQ-2 Foreign partial metadata.** The syntax of foreign metadata marking partial APIs as non-tryable (Section 6.9.5) is not fixed by this RFC — it is defined by RFC-008.
- **OQ-3 Failure return surface for strict fallible functions.** For functions with success results (Section 6.4.2), the surface form of the failure return was an open question. The variants:
  1. `return error err` — a contextual keyword (v2, Section 6.4.2/6.4.3): the explicit intent "the success result does not exist"; implementable without type tracking; a drawback — redundant for error-only functions and alien to Go readers.
  2. `return _, err` — blank padding: the programmer explicitly spells "the success slot is empty"; Go-idiomatic; exactly matches §6.6.4 ("padding is not initialization"); requires allowing `_` in return values and the kernel semantics "blank = an invisible ABI slot".
  3. Type-driven `return err` — failure by the value's type (no keyword/blank): a drawback — requires type-identity tracking in the kernel (currently only nullability classes) and carries a silent-flip risk when signatures change; deferred until the type infrastructure exists.
  For error-only functions the question is closed: the canonical style is `return err` (Section 6.4.4), implemented.
  Resolved by the owner on 2026-09-21 (owner decision, accepted): variant 1 — `return error err` (Section 6.4.2 unchanged). Criteria: (a) the industrial reference of a dedicated failure form (Swift `throw`, Rust `Err(..)`) — failure paths are searchable and visible in review, whereas `return _, err` reads in Go as "the value exists but is discarded" — a direct contradiction of the invariant of Section 6.2.5; (b) `return _, err` requires a new return-position value `_` with semantics opposite to Section 6.6.5; (c) a parse-level form without type tracking (the precedent of the error-only slice); (d) uniformity with the allowed error-only form `return error err` (Section 6.4.4); (e) arity-invariance — the form does not change as the number of success results grows, unlike `_, …, err`.
- **OQ-4 Early-exit conditional binding (guard / let-else).** For the destructuring of a fallible call (Section 6.3), the owner proposed (2026-09-21) an early-exit form with a flat success path:

  ```anuy
  var data, err = Load(path) else {
      log("load failed")
      return error err
  }

  use(data)
  ```

  Candidate semantics: the else block MUST diverge (`return`, `return error`, `break`, `continue`); on the fall-through path the controlling error is proven `nil` and the success bindings are definitely initialized — the form degenerates into the Section 6.3 correlation with syntactically required divergence (stronger than the inferred terminated-join of §6.3.2); inside the else, the controlling error is narrowed to non-null (D-5 clean), and consuming err in the else is compatible with must-consume (Section 6.6). Properties: the success path stays flat — unlike `if var` (RFC-021 §6.12: an inversion of the main-path indentation; the form was dropped in v3 of RFC-021 and v2 of RFC-022); composition with `try`: `try` remains the minimal propagation form, and guard-var covers propagate-with-context and non-propagation early exits (`break`/`continue` in loops). Surface variants: 1. `var … else` — an extension of the var declaration (the Rust let-else analogue), minimal grammar, the bindings keep living in the current scope; 2. `guard var … else` — a dedicated keyword (the Swift guard analogue), greppable, but a new form for the decomposable case. Implementation is a separate grammar slice over the correlation machinery, coordinated with RFC-003 (declaration and scope area); it is not part of the errors v1 core. The decision is made by the owner after the 005 core is closed; adopting the form does not change Sections 6.3/6.4 — it is an addition on top of them. Generalization (recorded 2026-09-21 after the owner's review, construct economy): `var … else` is proposed as the **only** conditional-binding form of the language — applicable to any witness-producing source: fallible calls (witness `error?`, Section 6.2.3), safe assertions (witness `bool`, RFC-021 §6.13; v3 of RFC-021 dropped `if var` in favor of this composition), exact channel receives (witness `ok`, RFC-022 v2 — with the synchronized removal of `if var value = <-ch`), and in perspective comma-ok map lookups (RFC-024 already uses the flat form). The witness MAY be spelled (`var data, err = Load() else { … }`) or absorbed (`var file = value.(File) else { … }` — the witness is consumed by the control flow, the `try` precedent). The kernel basis is the same: witness-guarded conditional initialization; acceptance closes the MAY condition of RFC-021 §6.12 and the closure criterion OQ-2 of RFC-022 ("a single formal protocol").

## 13. Normative Summary

RFC-005 v1 fixes:

1. `error` — a non-null Go-compatible error interface.
2. The ordinary absence of an error is represented as `error? = nil`.
3. A native function with a trailing `error?` is fallible.
4. The preceding results are the success results.
5. Native fallible functions are strict by default.
6. On failure, native success results have no observable Anuy values.
7. Explicit destructuring produces conditionally initialized success bindings.
8. `err == nil` proves the corresponding success bindings initialized.
9. Reassigning the controlling error kills the correlation.
10. A success return uses the ordinary `values..., nil`.
11. A failure with success results uses `return error err`.
12. A failure return requires a non-null error.
13. `try` propagates the trailing `error?`.
14. `try` strips the error result and yields the success results.
15. `try` evaluates the operand exactly once.
16. `try` propagates the original error unchanged.
17. `try` requires an enclosing fallible function.
18. `try` is allowed only in explicit propagation positions.
19. Nested `try` inside arbitrary expressions is forbidden.
20. Fallible results cannot be silently ignored.
21. Intentional ignore uses `discard`.
22. The blank identifier cannot bypass error checking.
23. Direct fallible tail forwarding through `return call()` is permitted when the protocols are compatible.
24. `defer` of a fallible call requires an explicit policy.
25. `defer discard call()` is allowed.
26. `defer try` is absent in v1.
27. A direct goroutine launch may not silently discard errors.
28. `try` does not interact with panic/recover.
29. No automatic error wrapping.
30. No automatic logging.
31. No core `Result<T,E>`.
32. No exceptions.
33. No postfix error `?`.
34. Native strict failure success slots lower to synthetic Go ABI padding.
35. ABI padding is not an Anuy default value.
36. Safe Anuy code cannot observe strict failure padding.
37. Foreign Go may have partial/raw fallible contracts.
38. The exact foreign classification belongs to RFC-008.
39. Error propagation uses the ordinary Go calling convention.
40. Synthetic propagation maps back through the `SourceMap`.
41. A strict fallible function must not fall off the end (missing return); an error-only fall-off is an implicit success `nil` (Section 6.4.7).
42. A mixed native return `value, err` (the trailing operand is not the literal `nil`) is rejected as D-7 (Sections 6.4.5, 8.1.7).
43. Destructured success results are conditionally initialized: guard = (controlling error == nil); reads without proof are rejected as D-4 (Sections 6.3, 8.1.4).
44. Correlation materialization does not escape a loop body; a controlling-error reassignment kills the correlation (Section 6.3.7).

## 14. References

### 14.1 Normative

- RFC-000 — Goals, Philosophy, Brand and Non-goals (the design filter);
- RFC-001 — Values, Initialization and Trust (conditional initialization);
- RFC-002 — Nullability and Nil Safety (`error?`, flow narrowing);
- RFC-003 — Variables, Assignment and Scope (multi-value declaration/assignment);
- RFC-004 — Interfaces and Explicit `impl` (error interface conformance);
- RFC-007 — Unsafe and Foreign Contracts (trusting foreign error values);
- RFC-008 — Go Interoperability (strict/partial foreign contracts, typed nil);
- RFC-009 — Lowering and Generated Go Contract (synthetic naming, ABI padding);
- RFC-011 — Diagnostics, Debugging and Tooling (SourceMap, codes, coverage).
