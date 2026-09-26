# RFC-019 — Functions, Closures and Calls

**Status:** Accepted
**RFC:** 019
**Title:** Functions, Closures and Calls
**Language:** Anuy
**Area:** Functions / Closures / Calls / Evaluation Order / Methods
**Version:** 7
**Date:** 2026-09-27
**Requires:** RFC-000, RFC-001, RFC-002, RFC-003, RFC-004, RFC-005, RFC-007, RFC-008, RFC-009, RFC-011, RFC-014, RFC-015, RFC-016, RFC-018
**Supersedes:** —

---

## Decision Record

RFC-019 was reviewed against RFC-000/001/002/003/004/005/007/008/009/011/014/015/016/018: it preserves Go-oriented syntax, definite initialization, universal nullability, lexical binding identity, the ordinary Go method model, explicit error propagation, foreign-boundary validation, direct Go interoperability, plain-Go lowering, source-oriented tooling, nominal types, Go-compatible visibility, bounded generics, and explicit conversions.

When designing, Go, Dart and Rust were considered:

* Go is the primary baseline for function declarations/types/literals, multiple results, variadics, method values, method expressions, and closure representation;
* Dart is prior art for first-class functions, lexical closures, function tear-offs, and optional/named parameter design;
* Rust is prior art for the fully determined left-to-right evaluation order of call operands and explicit closure-capture distinctions.

Owner decisions of the project, recorded in the Draft:

* Anuy keeps Go-compatible `func` syntax;
* function values are first-class values of the ordinary function type;
* a native function type is non-null by default;
* a nullable function is expressed with the ordinary RFC-002 `T?`;
* function declarations/literals have explicit parameter types;
* optional/default/named call parameters are absent in v1;
* multiple return values are preserved;
* multiple results are not a first-class tuple;
* named result parameters are forbidden in v1;
* a naked `return` in a result-bearing function is forbidden;
* all returned values are spelled explicitly or via a forwarding call;
* parameter passing preserves Go value semantics;
* parameters are initialized local bindings and MAY be reassigned;
* function calls have a fully determined left-to-right evaluation order;
* the callee/receiver is evaluated exactly once before the arguments;
* every argument is fully evaluated exactly once, left to right;
* closures use lexical implicit captures;
* a captured variable keeps binding identity and shared mutable storage, not a snapshot value;
* closure creation does not read the capture automatically;
* definite-initialization requirements of captured bindings are tracked until invocation/escape;
* an escaping closure must not leave open the possibility of a future read of a binding that may later become unavailable again;
* anonymous functions use Go-compatible `func(...) ... { ... }`;
* capture lists, `move` closures, and arrow syntax are absent in v1;
* variadic functions use Go-compatible `...T`;
* a variadic parameter has semantic type `[]T` inside the body;
* method values `x.M` and method expressions `T.M` preserve Go semantics;
* a method-value receiver is evaluated and saved exactly once when the function value is created;
* function type assignability uses no variance/subtyping;
* a generic function must be instantiated before use as a function value;
* calls through a type parameter are allowed only when the constraint guarantees a single callable signature;
* foreign callback invocation is a new foreign boundary of RFC-007/008.

Section 12 assigns every remaining question before `Accepted` to its responsible RFC or work stream.

Version 2, 2026-09-26: function types in the type grammar (story 01-69, implementing F-68-4 — function types were not accepted in any type position: `cb func(c Color) int` in a function declaration was rejected). §6.2: a function type is legal in every type position of the grammar; `func` is disambiguated by syntactic position — a type slot introduces a type without a body, an expression slot a literal with a body. §6.3: the `?` binding rule is normative — a trailing `?` after a single result belongs to the result; `?` directly after a result-list paren is invalid; the nullable whole-function spelling is `(func ...)?` only; the nullable function representation is native Go nil (RFC-002 §6.8.8), `== nil`/`!= nil` lower to the plain Go comparison (RFC-009 §6.7.12).

Version 3, 2026-09-26: kernel-enforced function types (story 01-70). §6.16: assignability is checked by the kernel as a source-first diagnostic before lowering; the go type-check stage (§6.4.8 RFC-010) remains a backstop, not the checker of record. Nullability participates only as the outer RFC-002 widening: a non-null function value is assignable to a nullable function type implicitly; nullable → non-null requires a narrowing proof. §8: the `ANUY19xx` numbers are the document-local order of the semantic catalog, recorded before the registry block scheme became normative; at activation a code is assigned per the RFC-011 §6.2.15 block scheme (the registry is the single source of truth).

Version 4, 2026-09-26: variadics in the restricted grammar (story 01-71, §6.14). §6.14 extended: the nullability of the variadic element follows the ordinary element rules — `...int?` is a variadic parameter of nullable `int?` elements; the spread requirement (an initialized non-null slice) applies to the slice value itself, not to its elements; individual variadic arguments widen to the element type per RFC-002 (`T → T?` when the element is nullable). The finality rules, the `[]T` body type, and identity (variadicness distinguishes `func(...int)` from `func([]int)`) stand — the slice pins them, it does not change them.

Version 5, 2026-09-26: method values and method expressions in the restricted grammar (story 01-72, §6.20–6.21). §6.20 extended: safe navigation does not form method values — a safe segment guards a safe CALL (RFC-002 §6.5.8); `c?.M` as a value is rejected, the non-null method value is obtained by narrowing the receiver. A method value's signature is the method's signature without the receiver; a method expression's signature takes the receiver as the first parameter (§6.21); both participate in ordinary assignability/identity (§6.2, §6.16).

Version 6, 2026-09-26: result list spellings (story 01-74, §6.5). §6.5 extended: `(T)` is the parenthesized single result, identical to `T`; `(T)?` makes the single result nullable, identical to `T?` — the spelling is REQUIRED exactly when the result type itself needs parentheses (composite types — the nullable whole-function result `(func() int)?`, RFC-002 §6.5.7); a list without a trailing `error?` is an ordinary non-fallible multi-result (fallibility stays keyed to the trailing `error?`, §6.22); `?` directly after a multi-result list is rejected — a result list is not a type. The drift is closed: the §6.5 example `func Split(value string) (string, string)` was unimplementable in the restricted grammar.

Version 7, 2026-09-27: **Accepted by the owner** (ADR-0015). The §8 catalog mapping to the RFC-011 registry (§8.21) is normative. Semantic changes: none.

---

## 1. Abstract

RFC-019 defines function declarations, function types, function values, anonymous functions, lexical closures, calls, multiple results, variadics, method values, and method expressions in Anuy.

The surface syntax preserves Go as much as possible, but call evaluation order is fully determined: the callee/receiver and arguments are evaluated exactly once, left to right. Native function values are non-null by default and follow the general nullable model of RFC-002.

Anuy keeps the Go lexical closure model with shared captured bindings, but integrates captures with definite initialization. Named result parameters and naked returns are not carried over from Go, because their zero-initialization semantics conflict with RFC-001.

## 2. Solutions

> **Anuy MUST use Go-compatible `func` syntax for function declarations, types, and literals.**

> **A native function type MUST be non-null by default.**

> **A nullable function value MUST be expressed with the ordinary RFC-002 nullable type.**

> **Parameters MUST have explicit static types.**

> **Optional, default, and named call parameters MUST NOT be supported in v1.**

> **Functions MAY return several values.**

> **A multiple result list MUST NOT be a first-class tuple type.**

> **Named result parameters MUST NOT be supported in v1.**

> **A result-bearing function MUST return values explicitly or via a compatible forwarding call.**

> **A naked `return` MUST NOT return hidden result bindings.**

> **Arguments MUST be passed by value under Go-compatible semantics.**

> **Function parameters MUST be initialized local bindings.**

> **Parameters MAY be reassigned; reassignment MUST NOT reassign the caller's binding.**

> **Call evaluation MUST be deterministic: callee/receiver first, then arguments left-to-right.**

> **Every callee, receiver, and argument expression MUST be evaluated exactly once.**

> **Function literals MUST be lexical closures.**

> **Captured variables MUST keep shared binding identity, not snapshot semantics.**

> **A closure MUST NOT be invoked or escape with the possibility of reading a captured binding that is not definitely initialized yet.**

> **Closure storage lifetime MUST be extended automatically on escape.**

> **A variadic parameter MUST be last and use `...T`.**

> **A variadic parameter inside the function body MUST have semantic type `[]T`.**

> **A method value `x.M` MUST evaluate and save the receiver exactly once when the method value is created.**

> **A method expression `T.M` MUST produce an ordinary function value with an explicit receiver parameter.**

> **Function type assignment MUST NOT apply parameter contravariance or result covariance.**

> **A generic function MUST be instantiated before ordinary function-value use.**

> **Foreign callback invocation MUST be treated as a foreign boundary.**

## 3. Motivation

### 3.1 The Go function model already fits Anuy well

Go functions already have most of the properties Anuy wants to keep:

* statically typed signatures;
* first-class function values;
* multiple results;
* lexical closures;
* variadics;
* methods;
* method values;
* method expressions;
* direct runtime support without a separate object model.

A Go function value already carries a reference to the function and the captured variables. Go function literals are lexical closures, and captured variables are shared between the outer scope and the closure.

RFC-019 therefore does not create a separate closure hierarchy or a callable protocol.

### 3.2 Named results conflict with RFC-001

Go allows:

```go
func Parse() (value int, err error) {
    ...
    return
}
```

Named result parameters are ordinary variables, automatically zero-initialized on function entry. A naked `return` then returns their current values.

For Anuy this means a hidden:

```text
value = 0
err = nil
```

on every entry.

This directly contradicts:

```text
uninitialized ≠ zero
```

from RFC-001.

One could keep the syntax but make the result bindings initially uninitialized and check them with dataflow. That, however, brings additional complexity:

* naked returns;
* partial result initialization;
* `defer`;
* panic/recover;
* result mutation from deferred functions;
* shadowing named results.

Anuy instead keeps multiple results but requires explicit return values.

### 3.3 Evaluation order must be fully deterministic

Go guarantees lexical ordering of function/method calls and some side-effecting operations, but the general order of evaluating subexpressions is not always fully determined. The current specification directly gives examples where the order of reading ordinary operands relative to function calls can affect the result.

For a source language with stronger invariants, the simpler rule is preferable:

```text
callee
arg1
arg2
arg3
call
```

fully left to right.

Rust applies a similar rule for call expressions: operands of multi-operand expressions are evaluated left to right.

This makes side effects predictable and simplifies review of generated code.

### 3.4 Closures and initialization need a separate rule

A Go closure can capture a local variable and outlive its lexical scope.

Anuy must also support:

```anuy
var count = 0

var increment = func() int {
    count = count + 1
    return count
}
```

But RFC-001 raises an additional question:

```anuy
var value int

var callback = func() {
    Use(value)
}
```

At the moment the closure is created, `value` may still be uninitialized.

Creating the closure `value` does not read it, so forbidding the capture automatically would be too strict: it would break recursive closures and useful initialization patterns.

However, the closure must not be allowed to be invoked, or escape to a place where it can read `value`, before its initialization.

RFC-019 therefore ties closure capture to definite-initialization dataflow.

## 4. Goals

1. Keep the familiar Go function syntax.
2. Keep first-class function values.
3. Keep multiple return values.
4. Keep lexical closures with shared mutable captures.
5. Keep Go-compatible parameter passing.
6. Make the evaluation order of calls fully deterministic.
7. Prevent implicit result initialization.
8. Align calls with the RFC-005 fallible function model.
9. Align function nullability with RFC-002.
10. Keep the Go variadic call model.
11. Keep method values and method expressions.
12. Support recursive local closures without abandoning RFC-001.
13. Do not introduce ownership/borrow semantics for closures.
14. Do not introduce a separate closure runtime.
15. Support generic instantiated functions as ordinary function values.
16. Ensure safe Go callback interop.
17. Keep simple function type identity without variance.
18. Ensure source-first diagnostics and debugger presentation.

## 5. Non-goals

1. RFC-019 does not introduce named/default parameters.
2. RFC-019 does not introduce optional positional parameters.
3. RFC-019 does not introduce Dart-style named arguments.
4. RFC-019 does not introduce arrow-function syntax.
5. RFC-019 does not introduce capture lists.
6. RFC-019 does not introduce a Rust-style `move` closure modifier.
7. RFC-019 does not introduce ownership/borrowing closures.
8. RFC-019 does not introduce function type variance.
9. RFC-019 does not introduce overload sets.
10. RFC-019 does not introduce user-defined callable objects/operator `()`.
11. RFC-019 does not introduce asynchronous functions; concurrency belongs to RFC-022.
12. RFC-019 does not define `defer`, panic, or recover; that is RFC-020.
13. RFC-019 does not define loop-variable capture behavior; that is RFC-023.
14. RFC-019 does not introduce first-class polymorphic function values; RFC-016 requires instantiation.
15. RFC-019 does not introduce generic function literals.
16. RFC-019 does not introduce named local function declarations in v1.
17. RFC-019 does not define the external C ABI; that is RFC-029.

## 6. Specification

### 6.1 Function declarations

A native function declaration MUST use the Go-compatible form:

```anuy
func Add(a int, b int) int {
    return a + b
}
```

Grouped parameter types MAY use the Go-compatible syntax:

```anuy
func Add(a, b int) int {
    return a + b
}
```

A function with no results:

```anuy
func Log(message string) {
    ...
}
```

A function with multiple results:

```anuy
func Split(value string) (string, string) {
    ...
}
```

Function parameters in a declaration with a body MUST have identifiers or `_`.

Parameters MUST have explicit types.

Anuy MUST NOT infer public/local declared function parameter types from the body.

A native function declaration MUST have a body.

Bodyless native declarations MUST NOT be used as an implicit foreign/assembly mechanism; foreign implementation mechanisms belong to RFC-008/029.

### 6.2 Function types

A function type MUST use the Go-compatible syntax:

```anuy
func(int) int
func(string, int) (string, error?)
func(values ...int) int
```

Parameter names in a function type MAY be present for documentation:

```anuy
func(value int, base int) string
```

or omitted:

```anuy
func(int, int) string
```

Parameter names MUST NOT participate in function type identity.

Result names MUST NOT be supported.

Valid:

```anuy
func() (int, error?)
```

Invalid:

```anuy
func() (value int, err error?)
```

Function type identity MUST take into account:

* the number of parameters;
* the ordered parameter types;
* variadicness;
* the number of results;
* the ordered result types.

Parameter names MUST NOT affect identity.

A function type MUST be legal in every type position of the grammar:

* the parameter type and the result type of a function declaration, a method declaration, or a function literal;
* a declared binding type:

```anuy
var cb func(c Color) int
```

* a field type:

```anuy
type Server struct {
    onLoad func(Event)
}
```

* the element type of composite types: `[]func(Event)`, `map[string]func(int) int`, `*func()`, as well as function-typed parameters of the function type itself.

The `func` keyword MUST be disambiguated by syntactic position, without additional syntax: in a type position `func` introduces a function type; in an expression position `func` introduces a function literal (Section 6.9), which MUST have a body. A function type has no body.

### 6.3 Function nullability

A native function type is non-null by default.

Example:

```anuy
var callback func(int) int
```

declares an uninitialized non-null function binding according to RFC-001.

It does not contain a semantic nil.

A nullable function MUST use ordinary RFC-002 nullability.

To avoid ambiguity with a nullable return type, the nullable whole-function type MUST be parenthesized:

```anuy
var callback (func(int) int)?
```

By contrast:

```anuy
func(int) int?
```

means:

```text
a non-null function
returning int?
```

A function declaration reference:

```anuy
Add
```

has a non-null function type.

A function literal also produces a non-null function value.

Calling a nullable function MUST require RFC-002 narrowing.

Example:

```anuy
if callback != nil {
    var result = callback(1)
}
```

RFC-019 introduces no separate safe-call operator in v1.

Function values MUST NOT support general equality.

A nullable function MAY compare to `nil` according to RFC-018.

Nullability suffix binding rules for function types:

* a trailing `?` after a single result type MUST bind to the result: `func() int?` is a non-null function returning `int?`;
* `?` MUST NOT stand directly after the closing paren of a result list: `func() (int, error?)?` is invalid; the nullability of individual results is spelled inside the list;
* the nullable whole-function type MUST be spelled with the full wrapper `(func(int) int)?` — the canonical formatting of RFC-002 §6.5.7.

A nullable function value uses the native Go nil representation (RFC-002 §6.8.8): no carrier type is introduced, `== nil`/`!= nil` lower to the plain Go comparison (RFC-009 §6.7.12), and narrowing before invocation follows RFC-002.

### 6.4 Parameters

Function parameters MUST become initialized local bindings before the function body begins.

Parameter passing MUST use Go-compatible value semantics.

For value types, the callee receives a value copy.

For pointers, slices, maps, channels, functions, and interfaces, the copied value MAY reference shared underlying state according to the owning RFC.

Reassigning a parameter:

```anuy
func Normalize(value int) int {
    value = value + 1
    return value
}
```

MUST be permitted.

Reassigning a parameter MUST NOT assign the caller's variable itself.

Mutating data reachable through a reference-like parameter MAY remain visible to the caller according to the corresponding type semantics.

Anuy v1 MUST NOT have:

```text
ref
out
inout
```

parameter modes.

Parameters MUST NOT have default values.

Invalid:

```anuy
func Connect(timeout int = 30) {
}
```

Parameters MUST NOT be optional merely due to nullability.

`T?` means a nullable required argument, not an optional omitted argument.

### 6.5 Results and explicit return

A function MAY have zero, one, or multiple results.

A multiple result list MUST remain a Go-style multi-value result and MUST NOT become a first-class tuple.

Example:

```anuy
func Position() (int, int) {
    return 10, 20
}
```

The expression:

```anuy
Position()
```

is multi-valued only in language contexts that support multiple values.

It MUST NOT be assignable to one ordinary variable as a hidden tuple.

Named result parameters MUST NOT be legal.

Invalid:

```anuy
func Position() (x int, y int) {
    ...
}
```

A function with result values MUST return all results explicitly:

```anuy
return x, y
```

A bare:

```anuy
return
```

MUST be legal only in a zero-result function.

Every reachable path of a result-bearing function MUST:

* execute a compatible `return`;
* diverge;
* panic/terminate through a language-defined non-returning operation.

Falling off the end of a result-bearing function MUST be a compile error.

Return expressions MUST evaluate exactly once, from left to right.

Each result MUST be assignable to the corresponding declared result type.

Result list spellings (grammar):

* `(T)` is the parenthesized single result - identical to `T`;
* `(T)?` makes the single result nullable - identical to `T?`; the spelling is REQUIRED exactly when the result type itself needs parentheses (composite types - the nullable whole-function result `(func() int)?`, per RFC-002 §6.5.7);
* a list without a trailing `error?` is an ordinary non-fallible multi-result - fallibility stays keyed to the trailing `error?` (§6.22, RFC-005);
* `?` directly after a multi-result list MUST be rejected - a result list is not a type (multiple results are not a first-class tuple).

### 6.6 Forwarding multiple results

Anuy MUST preserve Go-compatible direct result forwarding.

Example:

```anuy
func Outer() (int, string) {
    return Inner()
}
```

MUST be legal if `Inner()` returns exactly the compatible result list.

Forwarding MUST NOT materialize a first-class tuple.

RFC-005 governs fallible forwarding.

In particular, the direct compatible:

```anuy
return FallibleCall()
```

MAY remain valid according to RFC-005.

A multi-valued function call MAY serve as the entire argument list of another call when the result count/types match the parameters:

```anuy
Join(Split(value))
```

as in Go.

Such expansion MUST be allowed only when the inner call is the sole syntactic argument expression.

Fallible/error-bearing cases remain subject to the RFC-005 error-consumption rules.

### 6.7 Calls and argument checking

For:

```anuy
callee(arg1, arg2, arg3)
```

the compiler MUST verify:

* the callee has a callable function type;
* the argument arity matches;
* each argument is assignable to the corresponding parameter;
* variadic rules, if any, are satisfied.

The RFC-002 implicit widening:

```text
T → T?
```

MAY occur for parameter assignment.

RFC-018 untyped constant contextual typing MAY occur.

Other numeric/type conversions MUST NOT be inserted implicitly.

A nullable callee MUST NOT be invoked without narrowing.

A call's result type/value list is the function's declared result list after generic instantiation.

### 6.8 Call evaluation order

Call evaluation order MUST be fully deterministic.

For a plain call:

```anuy
makeFunc()(a(), b(), c())
```

evaluation MUST proceed:

1. fully evaluate `makeFunc()` to the callee value;
2. fully evaluate `a()`;
3. fully evaluate `b()`;
4. fully evaluate `c()`;
5. initialize the call parameters from the captured argument values;
6. begin the callee body.

Each step MUST complete before the next begins.

Each callee/argument expression MUST be evaluated exactly once.

For a method call:

```anuy
receiver().Method(a(), b())
```

evaluation MUST proceed:

1. fully evaluate the receiver expression;
2. resolve/save the effective receiver needed for the call;
3. fully evaluate `a()`;
4. fully evaluate `b()`;
5. invoke the method.

The compiler MAY introduce temporaries during lowering to preserve this order.

Lowering MUST NOT rely on Go operand-order freedom where it could change Anuy observable semantics.

### 6.9 Function literals

An anonymous function literal MUST use the Go-compatible syntax:

```anuy
func(value int) int {
    return value + 1
}
```

A function literal MAY be immediately invoked:

```anuy
var value = func(x int) int {
    return x * 2
}(21)
```

A function literal MUST NOT declare fresh type parameters.

Parameter types MUST be explicit.

The result rules of Section 6.5 apply equally to function literals.

Expression-bodied/arrow syntax:

```text
(x) => x + 1
```

MUST NOT be part of v1.

A function literal has the ordinary function type matching its signature.

The closure environment MUST NOT create a distinct source-visible nominal closure type.

### 6.10 Lexical capture semantics

A function literal MAY reference bindings from lexically enclosing function scopes.

Such a reference MUST create a capture of binding identity, not a copy of the value at closure creation.

Example:

```anuy
var count = 0

var next = func() int {
    count = count + 1
    return count
}

count = 10
var value = next()
```

`next()` observes the shared `count` binding.

A captured binding MUST remain shared between:

* the enclosing code;
* all closures capturing the same binding.

The compiler MUST preserve this semantics even when variable storage moves from stack-like to heap-like implementation.

Storage lifetime MUST automatically extend as long as any reachable closure may reference the captured binding.

Capture MUST NOT require explicit annotation.

Anuy v1 has no capture list.

### 6.11 Captures and definite initialization

Creating a closure MUST NOT itself count as reading the captured binding.

Therefore a function literal MAY syntactically capture a not-yet-initialized binding.

The compiler MUST compute, for each closure value, the set of captured bindings that may be read before being definitely assigned by the closure itself.

This set is the closure's **capture initialization requirements**.

A closure value MUST NOT be:

* invoked;
* returned;
* passed as an argument to another call;
* stored in an aggregate/container;
* stored in package/global state;
* passed across a foreign boundary;
* otherwise escaped from compiler-tracked local function-value flow;

unless every required captured binding is definitely initialized at that point.

A local assignment/copy between compiler-tracked function bindings MAY preserve pending requirements without requiring immediate satisfaction.

Example:

```anuy
var value int

var useValue = func() int {
    return value
}

value = 10

var result = useValue()
```

MUST be valid.

But:

```anuy
var value int

var useValue = func() int {
    return value
}

var result = useValue() // error
```

MUST be rejected.

### 6.12 Recursive closures

Capture-initialization analysis MUST permit recursive closure patterns without an implicit zero function value.

Example:

```anuy
var factorial func(int) int

factorial = func(n int) int {
    if n <= 1 {
        return 1
    }

    return n * factorial(n - 1)
}

var result = factorial(5)
```

During literal creation, the capture `factorial` has a pending initialization requirement.

The assignment of the closure to `factorial` initializes the binding.

Subsequent invocation therefore satisfies the requirement.

Mutually recursive local function bindings MAY similarly work if all pending capture requirements are satisfied before any invocation/escape.

Example:

```anuy
var even func(int) bool
var odd func(int) bool

even = func(n int) bool {
    if n == 0 {
        return true
    }
    return odd(n - 1)
}

odd = func(n int) bool {
    if n == 0 {
        return false
    }
    return even(n - 1)
}
```

Calling either before both required bindings are initialized MUST be rejected.

### 6.13 Escaping captured bindings

Once a closure with a read requirement for a captured binding has escaped into a context where the invocation timing is no longer statically controlled, the compiler MUST guarantee the captured binding remains semantically initialized for all future possible calls.

An operation that could transition such a captured binding from initialized to conditionally unavailable MUST be rejected after the escape.

Example classes include assigning an exact presence-correlated map lookup or a similar operation whose failure would make the binding unavailable.

Ordinary assignment of another valid initialized value remains permitted.

A closure assignment to a captured binding does not automatically improve outer definite-initialization facts unless ordinary flow analysis can prove the corresponding call occurred and the assignment is guaranteed.

Closure side effects MUST NOT be assumed merely because the closure value exists.

### 6.14 Variadic functions

A variadic function MUST use the Go-compatible syntax:

```anuy
func Sum(values ...int) int {
    ...
}
```

Only the final parameter MAY be variadic.

A function MUST have at most one variadic parameter.

Inside the body, a variadic parameter MUST have the semantic type:

```text
[]T
```

For:

```anuy
func Sum(values ...int)
```

`values` has the type:

```text
[]int
```

A call MAY pass zero or more individual arguments:

```anuy
Sum()
Sum(1)
Sum(1, 2, 3)
```

The compiler MUST construct a semantic slice containing exactly the supplied values.

Zero variadic arguments produce a valid empty semantic slice.

A call MAY pass a compatible slice using the Go-compatible spread:

```anuy
Sum(values...)
```

The spread argument MUST be the final argument.

The spread expression MUST be an initialized non-null slice.

Function type variadicness is part of type identity.

`func(...int)` and `func([]int)` MUST NOT be identical function types.

Nullability of the variadic element type follows the ordinary element rules: `...int?` is a variadic parameter of nullable `int?` elements, and the spread requirement (an initialized non-null slice) is on the slice value itself, not on its elements. Individual variadic arguments widen to the element type per RFC-002 (`T → T?` when the element is nullable); no other conversion is inserted implicitly.

### 6.15 Function values

A reference to a non-generic function declaration MUST produce a function value.

Example:

```anuy
var transform func(int) int = Double
```

A function value MAY be:

* assigned;
* passed;
* returned;
* stored in fields/collections;
* captured by a closure.

A native function value remains non-null unless explicitly widened to a nullable function type.

Function values MUST preserve the captured environment where applicable.

Built-in language operations MAY resemble calls but MUST NOT automatically be first-class function values unless the owning RFC explicitly defines a function type for them.

### 6.16 Function type assignability

Function values MUST be assignable only when the function signatures are identical after alias/identity rules, except the ordinary outer nullability widening.

No automatic function variance MUST exist.

Example:

```text
func(Dog) Animal
```

MUST NOT automatically be assignable to:

```text
func(Animal) Animal
```

or:

```text
func(Dog) Object
```

based solely on interface/subtyping-like relationships.

Parameter contravariance and result covariance MUST NOT be implicit.

The programmer MAY construct an explicit wrapper closure when adaptation is needed.

This preserves the Go-compatible direct function ABI and avoids hidden adapter allocation.

Assignability MUST be checked by the kernel as a source-level diagnostic before lowering; the go type-check stage (§6.4.8 RFC-010) remains a backstop, not the checker of record.

Nullability participates only as the outer widening per RFC-002: a non-null function value MUST be assignable to a nullable function type (`T → T?`); a nullable function value MUST NOT be assignable to a non-null function type without an RFC-002 narrowing proof.

### 6.17 Generic functions as values

A generic function MUST be instantiated before it becomes an ordinary function value, according to RFC-016.

Example:

```anuy
func Identity[T any](value T) T {
    return value
}

var f func(int) int = Identity[int]
```

MUST be valid.

A bare:

```anuy
var f = Identity
```

MUST NOT produce a first-class polymorphic function value unless the RFC-016 inference context fully instantiates it.

Calls may use RFC-016 generic inference.

A function literal MUST NOT itself be generic in v1.

### 6.18 Calls through type parameters

A type parameter value MAY be callable only if its RFC-016 constraint guarantees a single compatible function signature for every admissible type.

Example conceptual constraint:

```anuy
constraint IntTransform {
    ~func(int) int
}
```

MAY permit:

```anuy
func Apply[F IntTransform](f F, value int) int {
    return f(value)
}
```

If the admissible type set contains incompatible call signatures, the direct call MUST be rejected.

Call evaluation order remains Section 6.8.

### 6.19 Method calls

Direct method calls MUST continue to use the RFC-004/RFC-014 method-set and promotion rules.

Example:

```anuy
value.Method(arg)
```

Receiver selection, implicit address-taking/dereference where allowed, promoted method resolution, and explicit interface conformance remain governed by the owning RFCs.

The call argument/evaluation rules of RFC-019 apply after method resolution.

A nullable receiver MUST satisfy RFC-002 before a native method invocation.

Foreign nil-receiver behavior remains RFC-008.

### 6.20 Method values

The expression:

```anuy
value.Method
```

MUST produce a method value with the receiver bound.

The receiver expression MUST be evaluated exactly once at method-value creation.

The saved receiver MUST subsequently be used by every invocation of the resulting function value.

For a value receiver, the saved receiver is captured according to ordinary value-copy semantics.

For a pointer receiver, the saved receiver is the pointer value/reference.

Example:

```anuy
var f = user.Name
```

if `Name` has the signature:

```anuy
func (user User) Name(prefix string) string
```

then `f` has the type:

```anuy
func(string) string
```

A promoted method value MUST save the effective receiver selected through the embedding path.

Method value creation from a nullable receiver MUST require prior narrowing.

An interface method value MUST capture the interface value and perform the corresponding dispatch when invoked.

The method value itself is an ordinary non-null function value.

Safe navigation MUST NOT form a method value: a safe segment is the guard of a safe CALL (RFC-002 §6.5.8) — `c?.M(x)` is a nil-guarded invocation, while `c?.M` as a value MUST be rejected. Narrowing the receiver first yields the ordinary non-null method value.

### 6.21 Method expressions

Anuy MUST support Go-compatible method expressions:

```anuy
User.Name
(*Buffer).Write
```

If the method is:

```anuy
func (user User) Name(prefix string) string
```

then:

```anuy
User.Name
```

has the conceptually function type:

```anuy
func(User, string) string
```

The receiver is the explicit first parameter.

A method expression MUST NOT capture a receiver value.

Method-set/addressability rules MUST follow ordinary Go-compatible semantics constrained by RFC-004/014.

A method expression MAY itself be stored/passed as an ordinary function value.

### 6.22 Error-bearing functions

Function signatures MAY contain a trailing:

```text
error?
```

according to RFC-005.

Example:

```anuy
func Open(path string) (*File, error?) {
    ...
}
```

All strict success/failure correlation and `try` rules remain RFC-005.

RFC-019 MUST NOT reinterpret an error-bearing function as an exception-throwing function type.

The two signatures:

```text
func() Data
func() (Data, error?)
```

are distinct function types.

A function value with the second signature MUST NOT implicitly adapt to the first.

A function literal MAY be fallible using the same rules.

### 6.23 Call statements and discarded results

A zero-result function call MAY appear as a statement:

```anuy
Log("done")
```

A non-fallible function call with unused non-error results MAY appear in statement context if the general expression-statement grammar permits it, but the compiler SHOULD lint an obviously discarded pure/useful result where RFC-013 `must_use` or equivalent semantics apply.

If a call produces an error-bearing result governed by RFC-005, the error MUST NOT disappear silently.

The explicit:

```anuy
discard call()
```

remains the RFC-005 mechanism for intentional discard.

### 6.24 Foreign functions and callbacks

Imported Go function values MUST project according to RFC-008.

Because a Go function value may be nil, a foreign function value SHOULD project nullable unless the contract proves non-null.

A native function passed to Go MAY lower directly only when the boundary preserves all Anuy invariants.

If foreign Go code may invoke a native function later, this invocation MUST be treated as a new foreign boundary.

Incoming callback arguments MUST be projected/validated as foreign inputs.

A native callback result returned to Go MUST be lowered according to RFC-008/009.

The compiler MAY synthesize a wrapper function.

The wrapper MUST preserve:

* exactly-once argument evaluation at the original call site;
* callback identity/lifetime as required by the Go API;
* source diagnostics through SourceMap.

Passing a closure to a foreign call counts as escape for capture-initialization Section 6.11.

### 6.25 Lowering

An ordinary native function MUST lower to an ordinary Go function where the signature is representable.

A function literal MUST lower to an ordinary Go function literal/closure.

Captured bindings MAY lower using ordinary Go closure capture storage.

The compiler MUST preserve the Anuy deterministic call evaluation order.

Where the Go source expression order is insufficiently specified, the compiler MUST introduce temporaries.

Example conceptual source:

```anuy
f(a(), b(), c())
```

MAY lower approximately to:

```go
__anuy_f := f
__anuy_a := a()
__anuy_b := b()
__anuy_c := c()
__anuy_f(__anuy_a, __anuy_b, __anuy_c)
```

when needed to preserve exact semantics.

Synthetic temporaries MUST follow the RFC-009/011 SourceMap rules.

Multiple results MUST lower to ordinary Go multiple results.

No tuple allocation is required.

No closure object runtime beyond the ordinary Go closure representation is required.

Capture-init requirements are compile-time dataflow metadata and MUST NOT require runtime validity bits.


## 7. Interaction with Other RFCs

### RFC-000 — Goals, Philosophy, Brand and Non-goals

RFC-019 **depends on** RFC-000.

It preserves Go functions almost entirely and introduces stronger rules only where intent/correctness benefits materially:

* explicit results;
* deterministic evaluation;
* capture initialization.

### RFC-001 — Values, Initialization and Trust

RFC-019 **depends on and extends** RFC-001.

Parameters are initialized by the call.

Named result zero initialization is removed.

Closure captures participate in definite initialization.

A native function zero/nil backing is not a semantically initialized function value.

### RFC-002 — Nullability and Nil Safety

RFC-019 **depends on** RFC-002.

A function type is non-null by default.

A nullable function uses `T?`.

Calling a nullable function requires narrowing.

A function returning a nullable value and the nullable function itself remain distinct.

### RFC-003 — Variables, Assignment and Scope

RFC-019 **depends on** RFC-003.

Parameters and captures use binding identity.

Captured mutation targets the original binding.

Closure capture dataflow tracks `SymbolID`, not names.

### RFC-004 — Interfaces and Explicit `impl`

RFC-019 **depends on** RFC-004.

Method sets, receiver legality, and interface methods remain RFC-004.

Method values/expressions only expose already valid methods.

### RFC-005 — Error Handling and Propagation

RFC-019 **depends on** RFC-005.

Error-bearing function signatures remain ordinary multiple-result functions with stronger correlation.

Direct return/call forwarding preserves RFC-005.

No exceptions are introduced.

### RFC-007 — Unsafe and Foreign Contracts

RFC-019 **depends on** RFC-007.

Callbacks invoked by foreign code create a foreign provenance boundary.

Unsafe does not disable closure initialization checks.

### RFC-008 — Go Interoperability

RFC-019 **depends on** RFC-008.

Go function values may be nullable.

Foreign callback arguments/results require projection.

Function/method representation remains Go-compatible.

### RFC-009 — Lowering and Generated Go Contract

RFC-019 **depends on** RFC-009.

Functions/closures lower to ordinary Go.

Deterministic evaluation MAY require synthetic temporaries.

No custom call ABI is introduced.

### RFC-011 — Diagnostics, Debugging and Tooling

RFC-019 **depends on** RFC-011.

Synthetic call-order temporaries and callback wrappers remain hidden by default.

Closure capture state should be presented semantically.

### RFC-014 — Types, Structs, Embedding and Construction

RFC-019 **depends on** RFC-014.

Function fields are ordinary fields and must be completely initialized.

Method promotion participates in method value/expression resolution.

### RFC-015 — Packages, Visibility and Public API

RFC-019 **depends on** RFC-015.

Function/method visibility follows capitalization.

Function type parameter/result names do not affect visibility.

### RFC-016 — Generics and Constraints

RFC-019 **depends on** RFC-016.

Generic functions must instantiate before function-value use.

Calls through a type parameter require a guaranteed common function signature.

Function literals do not add fresh generics.

### RFC-018 — Constants, Literals, Operators and Conversions

RFC-019 **depends on** RFC-018.

Untyped constants may contextually adapt to a parameter type.

Runtime numeric conversions are never inserted implicitly into calls.

Function equality follows RFC-018.

### RFC-020 — Defer, Panic and Recover

RFC-019 **defers** exact deferred-call timing/result interactions to RFC-020.

RFC-020 MUST preserve the Section 6.8 argument evaluation guarantees.

Named result parameters remain absent, simplifying defer result mutation semantics.

### RFC-022 — Concurrency: Goroutines, Channels and Select

RFC-019 **defers** concurrent invocation and goroutine calls to RFC-022.

Captured variable races remain a Go memory-model/concurrency concern.

### RFC-023 — Iteration and Range Semantics

RFC-019 **defers** loop-variable capture semantics to RFC-023.

RFC-023 MUST preserve the binding-identity capture model defined here.

## 8. Diagnostics and Tooling

### 8.1 `ANUY1901` — Named result parameters are not supported

Condition:

The function/result signature contains result binding names.

Example:

```anuy
func Parse() (value int, err error?) {
}
```

Illustrative message:

```text
error[ANUY1901]: named result parameters are not supported

return values explicitly:
    func Parse() (int, error?)
```

### 8.2 `ANUY1902` — Missing return values

Condition:

A result-bearing function uses a bare `return`.

```text
error[ANUY1902]: this function must return 2 values explicitly
```

### 8.3 `ANUY1903` — Missing return path

Condition:

A reachable function path may fall off the end of a result-bearing function body.

Illustrative message:

```text
error[ANUY1903]: function may complete without returning `User`
```

### 8.4 `ANUY1904` — Call arity mismatch

Condition:

The argument count is incompatible with the signature/variadic rules.

```text
error[ANUY1904]: function expects 2 arguments but 3 were provided
```

### 8.5 `ANUY1905` — Argument type mismatch

Condition:

An argument is not assignable to the parameter.

The diagnostic SHOULD show the parameter position/name and the required/provided types.

### 8.6 `ANUY1906` — Nullable function call

Condition:

A nullable function value is invoked without narrowing.

```anuy
var callback (func(int))? = ...
callback(1)
```

Illustrative message:

```text
error[ANUY1906]: nullable function may be nil

narrow `callback` before calling it
```

### 8.7 `ANUY1907` — Captured binding not initialized

Condition:

A closure is invoked/escaped while a required captured binding is not definitely initialized.

Illustrative message:

```text
error[ANUY1907]: closure may read `value` before it is initialized

captured here:
    ...

initialize `value` before invoking or escaping this closure
```

### 8.8 `ANUY1908` — Escaped capture may become unavailable

Condition:

A binding captured by an already escaped closure is assigned through an operation that may transition the binding to an unavailable state.

Illustrative message:

```text
error[ANUY1908]: `value` must remain initialized because an escaping closure may read it
```

### 8.9 `ANUY1909` — Invalid variadic parameter

Condition:

A variadic parameter:

* is not last;
* is repeated;
* is malformed.

```text
error[ANUY1909]: variadic parameter must be the final parameter
```

### 8.10 `ANUY1910` — Invalid variadic expansion

Condition:

A `...` call argument:

* is not final;
* is not a compatible slice;
* is nullable without narrowing.

### 8.11 `ANUY1911` — Function type mismatch

Condition:

An assignment expects a function signature different from the actual value.

The diagnostic SHOULD explicitly note that Anuy does not apply implicit function variance.

### 8.12 `ANUY1912` — Generic function must be instantiated

Condition:

A generic function is used as a function value without sufficient instantiation/inference.

```text
error[ANUY1912]: generic function `Identity` requires instantiation before use as a function value
```

### 8.13 `ANUY1913` — Type parameter is not callable

Condition:

A call through `T` where the constraint does not guarantee a common callable signature.

### 8.14 `ANUY1914` — Invalid multi-value expansion

Condition:

A multi-valued call is used where:

* it is not the sole argument;
* the arity mismatches;
* the result types are incompatible.

### 8.15 `ANUY1915` — Default/optional parameters are not supported

Example:

```anuy
func Connect(timeout int = 30) {}
```

Illustrative message:

```text
error[ANUY1915]: function parameters cannot have default values
```

### 8.16 Completion

LSP SHOULD:

* show the function signature and result list;
* distinguish a nullable function from a function returning a nullable value;
* offer variadic spread only for a compatible slice;
* show captured variables for a closure;
* show the method-value resulting function signature.

For a nullable callable, completion MAY suggest nil narrowing.

### 8.17 Hover

Hovering a function value SHOULD show the exact Anuy type:

```text
func(int, string) (User, error?)
```

Hovering a closure MAY show:

```text
captures:
    count : int (shared)
```

Pending capture-initialization requirements SHOULD be visible in technical tooling mode.

### 8.18 Go-to-definition

A function value reference navigates to the function declaration.

A method value/expression navigates to the method declaration.

A generic instantiated function SHOULD navigate to the generic source declaration, not the generated instantiation.

### 8.19 Rename

Parameter/capture rename uses binding identity.

Renaming an outer captured binding MUST update closure references.

Result names do not exist as semantic bindings.

### 8.20 Debugging

The debugger MUST present closure captures as semantic variables, not compiler-generated environment fields.

Pending/uninitialized captured bindings MUST display as unavailable.

Synthetic call-order temporaries MUST be hidden by default.

A method value SHOULD display the bound receiver meaningfully where the debugger supports it.

### 8.21 Diagnostic codes and the RFC-011 registry

The `ANUY19xx` numbers of this Section are document-local ordering of the
semantic catalog, committed before the registry block scheme became
normative (RFC-011 §6.2.15, v3). At activation, a diagnostic takes its
code from the registry block of its subsystem — the registry is the
single source of truth. This Section remains the semantic catalog; it
does not pre-assign registry codes.

Accepted mapping of the implemented §8 semantics to the registry codes
(ADR-0015; the full result protocol of §6.7.1 RFC-005 verifies in the
kernel):

```text
§8.1  named result parameters   ANUY1001 (parser reject)
§8.2  missing return values     ANUY1006/ANUY1001 (parser frame)
§8.3  missing return path       ANUY3003
§8.4  call arity                ANUY1006/ANUY1001
§8.5  argument mismatch         ANUY4003/ANUY7008 + go type-check stage
§8.6  nullable call             ANUY4011 (statement and value positions)
§8.7  captured not initialized  ANUY3001-family (definite initialization)
§8.8  escaped capture           kernel flow facts — boundary, follow-up
§8.9  invalid variadic param    ANUY1001
§8.10 invalid expansion         ANUY1001 + go type-check stage (elements)
§8.11 function type mismatch    ANUY7008
§8.12 generic instantiation     RFC-016 (outside the v1 core)
§8.13 type parameter call       RFC-016 (outside the v1 core)
§8.14 multi-value expansion     ANUY1006/ANUY1001 (forwarding, story 74/76)
§8.15 parameter defaults        ANUY1001 (targeted reject)
```


## 9. Rationale

### 9.1 Why the Go `func` syntax is preserved

Here new syntax creates no additional safety.

Go's:

```go
func Add(a, b int) int
func(int) int
func(x int) int { ... }
```

already covers declaration, type, and closure.

Anuy keeps the familiar form and strengthens the dataflow/evaluation semantics.

### 9.2 Why there are no named results

Go named result parameters exist as real local variables and are zero-initialized on function entry. Naked returns use their current values.

It is exactly the zero initialization that makes this feature incompatible with RFC-001.

One could change only the initialization semantics, but then the Go-identical syntax would have surprisingly more complex behavior.

The explicit:

```anuy
return value, err
```

shows the actual function outcome better.

Go code-review guidance itself recommends not using named results just for naked returns and preferring explicit values outside short functions.

### 9.3 Why multiple results are not tuples

Go multiple results work well for:

* errors;
* map/channel presence;
* parsers;
* split values.

Turning them into a first-class tuple would change:

* assignability;
* the ABI;
* generic behavior;
* reflection;
* method signatures.

Anuy has no need for this extension in RFC-019.

### 9.4 Why parameters are mutable

Parameters could be immutable-by-default.

However, that would significantly increase the migration distance from Go and effectively introduce a new mutability model that local variables do not have yet.

Reassigning the parameter itself is safe: the argument has already been passed by value.

If a future RFC introduces general immutability, parameters must be revisited together with all bindings.

### 9.5 Why the call order is stricter than Go

Go guarantees significant lexical ordering but intentionally leaves some operand-order details unspecified.

That is good for compiler freedom but bad for source predictability.

Anuy generated Go can already introduce temporaries, so both worlds are achievable:

```text
source
    deterministic

backend
    ordinary Go
```

Rust demonstrates that a left-to-right operand order is practical as a language contract.

### 9.6 Why closures capture variables, not values

Go developers expect:

```anuy
var x = 1
var f = func() int { return x }
x = 2
```

and `f()` observes `2`.

Go function values explicitly retain references to the enclosed variables.

Snapshot-by-default semantics would drastically change migration behavior.

If the programmer needs a snapshot, they can make an explicit new binding:

```anuy
var snapshot = x

var f = func() int {
    return snapshot
}
```

### 9.7 Why there are no explicit capture lists

Swift/Rust-style capture controls are useful with ownership/value capture models.

Anuy keeps the Go GC/shared-variable model.

A capture list adds no necessary invariant and creates new syntax.

### 9.8 Why a capture may be created before initialization

A complete ban:

```text
captured binding must already be initialized at closure creation
```

would break the ordinary recursive closure pattern.

Creating the closure itself does not read the variable.

The risk appears only when the closure can execute.

The requirement is therefore tied to invocation/escape, not literal creation.

### 9.9 Why pending capture requirements are compile-time metadata

One could store a runtime initialization flag in the closure environment.

But RFC-001 requires static proof where possible.

Pending requirements are a dataflow fact, not runtime state.

The compiler checks them before invocation/escape, and the generated closure remains an ordinary Go closure.

### 9.10 Why the variadic parameter is `[]T`

This is exactly the Go mental model and directly compatible with `append`, `len`, iteration, and generated Go.

Since the Anuy `[]T` is semantically present even with a nil-backed Go representation, zero variadic arguments naturally produce an empty present slice.

### 9.11 Why there are no named/default parameters

Dart provides required positional, optional positional, and named parameters, including `required` named arguments and compile-time defaults.

That is convenient for UI/configuration-heavy APIs, but for Anuy it creates a significant divergence:

* Go functions have no such ABI;
* function type identity becomes more complex;
* wrappers are needed for Go publication;
* migrating APIs becomes less mechanical.

Go-style alternatives already exist:

* option structs;
* builder/config objects;
* explicit wrapper functions;
* functional options.

V1 therefore keeps positional required parameters.

### 9.12 Why there is no arrow syntax

Dart expression-bodied functions and Rust closures provide compact syntax.

But Anuy already has the concise Go function literal.

Adding:

```text
x => x + 1
```

creates no new semantic capability.

### 9.13 Why there is no function variance

Dart function types have developed subtyping rules; Rust closures have their own anonymous types/call traits.

Anuy does not need this complexity.

Exact function signatures:

* match Go;
* require no hidden adapters;
* are simpler for the ABI;
* are simpler for foreign callbacks.

An explicit wrapper closure solves the rare adaptation cases.

### 9.14 Why the method value saves the receiver immediately

The Go method value `x.M` evaluates and saves the receiver at method-value creation, while the method expression `T.M` leaves the receiver as an explicit argument.

This is a useful and well-known distinction.

Anuy preserves it literally, since it lowers directly and does not conflict with the stronger invariants.

## 10. Rejected Alternatives

### 10.1 Named result parameters with Go zero initialization

Rejected directly by RFC-001.

### 10.2 Named result parameters as uninitialized bindings

Rejected in v1.

Although a sound implementation is possible, the feature complicates:

* naked return;
* defer;
* definite initialization;
* shadowing;
* result mutation.

Value explicitness does not justify the complexity.

### 10.3 Named results only as documentation labels

Rejected.

The Go-compatible syntax would look like a binding declaration but have a different meaning.

Documentation is better expressed with doc comments.

### 10.4 Naked returns

Rejected, since without named result bindings they have nothing to return.

### 10.5 Unspecified/Go-like partial call evaluation order

Rejected in favor of predictable semantics.

### 10.6 Right-to-left argument evaluation

Rejected as unfamiliar to Go developers and contrary to industry practice.

### 10.7 Capture-by-value default

Rejected as a major semantic divergence from Go.

### 10.8 Mandatory initialized-at-creation captures

Rejected, because it breaks recursive/mutually-recursive closure patterns.

### 10.9 Runtime capture validity flags

Rejected, since static dataflow is sufficient and runtime overhead is unnecessary.

### 10.10 Explicit capture lists

Rejected from v1.

May return only together with a broader mutability/concurrency/ownership model.

### 10.11 `move` closures

Rejected for the same reason.

### 10.12 Dart-style named parameters

Rejected from the core v1 due to Go compatibility/migration/ABI cost.

### 10.13 Default parameter values

Rejected.

Wrappers/config structs are sufficient and match Go better.

### 10.14 Arrow closures

Rejected as unnecessary syntax duplication.

### 10.15 Local named function declarations

Alternative:

```anuy
func local(x int) int {
    ...
}
```

inside a block.

Rejected in v1 to keep the Go declaration grammar.

Recursive anonymous closures are already supported by Section 6.12.

### 10.16 Closure-specific anonymous nominal types

Rejected.

The Go-compatible ordinary function type is sufficient.

### 10.17 Function type variance

Rejected due to hidden adaptation and ABI complexity.

### 10.18 First-class generic function values

Rejected by RFC-016.

Concrete instantiation required.

### 10.19 Implicit nullable function call

Alternative:

```anuy
callback(args)
```

silently no-op when the callback is nil.

Rejected, because hidden control flow violates RFC-002.


## 11. Prior Art

### 11.1 Go

Go is the primary baseline of RFC-019.

Go function types use `func`, support multiple results and variadic parameters. Function literals are closures, and enclosed variables are shared and retained as long as accessible. A method value saves the receiver when the expression is evaluated; a method expression exposes the receiver as the first ordinary argument.

Anuy keeps:

```text
func syntax
first-class function values
multiple results
variadics
closures
shared captures
method values
method expressions
```

and differs in three main places:

```text
named results
    Go: supported, zero-initialized
    Anuy: absent

call evaluation
    Go: partially unspecified in some expression interactions
    Anuy: fully left-to-right

captures + initialization
    Go: every variable has a zero value
    Anuy: definite-init requirements tracked
```

### 11.2 Dart

Dart functions are first-class objects; it supports lexical closures, function types, anonymous functions, and tear-offs. Dart also supports required positional, optional positional, named parameters, and defaults.

For a team familiar with Dart:

```text
Dart tear-off
≈
Anuy/Go function reference or method value
```

For example, the conceptual:

```dart
buffer.write
```

and:

```anuy
buffer.Write
```

both create a callable with a bound receiver.

Anuy deliberately does not carry over Dart optional/named parameters, since the Go ABI/API model remains positional.

Dart lexical closures confirm the familiar shared-environment model, but Anuy does not make functions universal objects/classes.

### 11.3 Rust

Rust closures have unique anonymous closure types and can choose the capture mode depending on use, including `move`; call capabilities are shared through `Fn`, `FnMut`, `FnOnce`.

Anuy does not borrow this, because:

* there is no ownership model;
* the Go runtime closure representation already fits;
* the ordinary function type is simpler for interoperability.

But Rust is useful prior art for deterministic evaluation: call-expression operands are evaluated left to right.

### 11.4 Comparison summary

RFC-019 chooses:

```text
Go
    syntax
    multiple results
    variadics
    closure representation
    method values/expressions

Dart
    first-class-function / tear-off ergonomics as a familiar comparison

Rust
    deterministic operand evaluation as a safety precedent
```

with the Anuy-specific additions:

```text
no zero-initialized result bindings

closure capture
+
definite initialization
```

## 12. Open Questions

### OQ-1 — `defer` interaction with explicit results

**Question:** Can a deferred function affect the result values without named result variables, and how exactly are defer arguments evaluated?

**Owner:** RFC-020 — Defer, Panic and Recover.

**Closure criterion:** RFC-020 defines the defer lifecycle without returning named-result zero semantics.

**Blocks Accepted:** No, for ordinary function/call semantics.

### OQ-2 — Loop capture

**Question:** What binding identity is created for loop/range variables per iteration, and what does a closure capture?

**Owner:** RFC-023 — Iteration and Range Semantics.

**Closure criterion:** RFC-023 fixes the per-iteration binding behavior.

**Blocks Accepted:** No.

### OQ-3 — Safe invocation of a nullable function

**Question:** Is a concise safe-call syntax needed for:

```text
(func(...))?
```

analogous to safe navigation?

**Owner:** RFC-002 follow-up / Language Core work stream.

**Closure criterion:** Real use cases show sufficient frequency to make new syntax better than the explicit nil check.

**Blocks Accepted:** No. Narrowing fully covers the semantics.

### OQ-4 — Future named/default arguments

**Question:** Are Go-style config/options APIs sufficient, or do named arguments materially improve large Anuy APIs?

**Owner:** Post-v1 Language Ergonomics work stream.

**Closure criterion:** Production experience shows a recurring correctness/readability problem not solvable by structs/wrappers without excessive ceremony.

**Blocks Accepted:** No.

## 13. Normative Summary

1. Functions use Go-compatible `func` syntax.
2. Native function declarations require bodies.
3. Function parameters have explicit types.
4. Grouped Go-style parameter types MAY be used.
5. Functions MAY have zero results.
6. Functions MAY have one result.
7. Functions MAY have multiple results.
8. Function types use `func`.
9. Function-type parameter names MAY be present or absent.
10. Parameter names do not participate in function type identity.
11. Result names are not supported.
12. Function type identity includes parameter count/order/types.
13. Function type identity includes result count/order/types.
14. Function type identity includes variadicness.
15. Native function type is non-null by default.
16. Nullable function uses RFC-002.
17. Nullable whole-function type requires unambiguous grouping.
18. Function returning nullable value is distinct from nullable function.
19. Function declaration reference produces non-null function value.
20. Function literal produces non-null function value.
21. Nullable function call requires narrowing.
22. General function equality absent.
23. Nullable function MAY compare to nil.
24. Function parameters are initialized before body.
25. Parameter passing is by value.
26. Reference-like parameter values MAY share underlying state.
27. Parameters MAY be reassigned.
28. Parameter reassignment does not reassign caller binding.
29. Anuy has no ref/out/inout parameters in v1.
30. Parameters have no defaults.
31. Parameters are not optional due nullability.
32. Multiple results are not first-class tuple.
33. Named result parameters are forbidden.
34. Result-bearing function must return explicit values or forwarding call.
35. Bare return only valid in zero-result function.
36. Every reachable result-bearing path must return/diverge.
37. Falling off result-bearing function is error.
38. Return expressions evaluate exactly once.
39. Return expressions evaluate left-to-right.
40. Each returned value must match declared result type.
41. Direct multi-result return forwarding is supported.
42. Forwarding does not create tuple.
43. Multi-result call MAY expand as sole argument list.
44. Multi-result expansion requires arity/type compatibility.
45. Error-bearing expansion follows RFC-005.
46. Call verifies callable type.
47. Call verifies argument arity.
48. Call verifies argument assignability.
49. Nullable widening MAY apply to parameters.
50. Untyped constant contextual typing MAY apply.
51. Other conversions are not implicitly inserted.
52. Callee expression evaluates first.
53. Callee expression evaluates exactly once.
54. Method receiver expression evaluates first.
55. Method receiver evaluates exactly once.
56. Arguments evaluate left-to-right.
57. Each argument fully evaluates before next.
58. Each argument evaluates exactly once.
59. Parameters initialize after argument values are obtained.
60. Compiler MAY insert temporaries to preserve order.
61. Function literals use Go-compatible syntax.
62. Function literals MAY be immediately invoked.
63. Function literals cannot declare fresh type parameters.
64. Function-literal parameter types are explicit.
65. Arrow syntax absent in v1.
66. Function literal may capture enclosing bindings.
67. Capture uses binding identity.
68. Capture does not snapshot value by default.
69. Captured binding is shared with enclosing scope.
70. Captured binding is shared across closures capturing same binding.
71. Captured storage lifetime extends while reachable closure references it.
72. Capture requires no explicit annotation.
73. Capture lists absent in v1.
74. Closure creation does not itself read capture.
75. Compiler computes capture initialization requirements.
76. Requirement covers captures potentially read before closure-local assignment.
77. Closure cannot be invoked with unsatisfied capture requirements.
78. Closure cannot escape with unsatisfied capture requirements.
79. Passing closure as argument counts as escape.
80. Returning closure counts as escape.
81. Storing closure in aggregate/container counts as escape.
82. Storing closure in package/global state counts as escape.
83. Foreign callback passage counts as escape.
84. Local compiler-tracked function binding MAY retain pending requirements.
85. Recursive closure via initially uninitialized function binding MAY be valid.
86. Recursive closure call requires self-binding initialized.
87. Mutually recursive closures MAY be valid.
88. Calling mutually recursive closure before requirements satisfied is error.
89. Escaped closure captures that may be read must remain initialized.
90. Operation making such capture conditionally unavailable is rejected.
91. Ordinary valid reassignment of escaped capture remains allowed.
92. Closure side effects do not automatically establish outer initialization facts.
93. Variadic parameter uses `...T`.
94. Only last parameter MAY be variadic.
95. Function has at most one variadic parameter.
96. Variadic parameter body type is `[]T`.
97. Zero variadic args produce valid empty slice.
98. Individual variadic args supported.
99. Slice expansion with `...` supported.
100. Spread argument must be final.
101. Spread requires compatible initialized slice.
102. Variadicness participates in function type identity.
103. Function declaration reference is first-class value.
104. Function values MAY be assigned.
105. Function values MAY be passed.
106. Function values MAY be returned.
107. Function values MAY be stored.
108. Function values MAY capture environment.
109. Built-ins are not automatically first-class function values.
110. Function assignability uses exact signatures.
111. Function variance absent.
112. Hidden function adapters absent.
113. Generic function must instantiate before ordinary function-value use.
114. Generic inference MAY instantiate from context according RFC-016.
115. Function literals are not generic.
116. Type-parameter call requires constraint with common callable signature.
117. Method calls follow RFC-004/014 method resolution.
118. Method call obeys RFC-019 evaluation order.
119. Nullable native receiver requires narrowing.
120. Method value binds receiver.
121. Method-value receiver evaluates once at creation.
122. Value-receiver method value saves value according to copy semantics.
123. Pointer-receiver method value saves pointer.
124. Promoted method value saves effective receiver.
125. Interface method value captures interface value.
126. Method value is ordinary function value.
127. Method expression exposes receiver as first argument.
128. Method expression captures no receiver.
129. Method expression MAY be stored/passed.
130. Error-bearing signatures remain ordinary function types under RFC-005.
131. Fallible function does not throw exception.
132. Fallible and non-fallible signatures are distinct.
133. Error-bearing function value does not implicitly adapt to non-fallible type.
134. Non-error function results MAY be discarded subject to lints.
135. Error results must follow RFC-005 discard/handling rules.
136. Foreign Go function values project under RFC-008.
137. Foreign Go function values MAY be nullable.
138. Native callback passed to Go MAY require wrapper.
139. Foreign invocation of native callback is foreign boundary.
140. Callback arguments are reprojected as foreign inputs.
141. Callback results lower according RFC-008/009.
142. Ordinary functions lower to ordinary Go functions.
143. Function literals lower to ordinary Go closures.
144. Multiple results lower to ordinary Go multiple results.
145. No tuple allocation required.
146. No custom closure runtime required.
147. Deterministic evaluation may require synthetic temporaries.
148. Synthetic temporaries follow RFC-009/011 mapping.
149. Capture requirements are compile-time metadata.
150. Capture requirements require no runtime initialization flags.

## 14. References

### 14.1 Normative

* RFC-000 — Goals, Philosophy, Brand and Non-goals
* RFC-001 — Values, Initialization and Trust
* RFC-002 — Nullability and Nil Safety
* RFC-003 — Variables, Assignment and Scope
* RFC-004 — Interfaces and Explicit `impl`
* RFC-005 — Error Handling and Propagation
* RFC-007 — Unsafe and Foreign Contracts
* RFC-008 — Go Interoperability
* RFC-009 — Lowering and Generated Go Contract
* RFC-011 — Diagnostics, Debugging and Tooling
* RFC-014 — Types, Structs, Embedding and Construction
* RFC-015 — Packages, Visibility and Public API
* RFC-016 — Generics and Constraints
* RFC-018 — Constants, Literals, Operators and Conversions
* RFC 2119 — Key words for use in RFCs to Indicate Requirement Levels
* RFC 8174 — Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words

### 14.2 Informative

* Go Language Specification — function types, declarations, literals, calls, closures, method values, method expressions, and evaluation order.
* Go Code Review Comments — named result parameters and naked-return guidance.
* Dart language documentation — first-class functions, function types, lexical closures, tear-offs, named/default/optional parameters.
* Effective Dart — preference for declarations when naming local functions and direct tear-offs instead of unnecessary lambdas.
* Rust Reference — closure expressions and capture model.
* Rust Reference — deterministic left-to-right operand evaluation.
