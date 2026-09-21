package integration

import (
	"sort"
	"strings"
	"unicode"

	"github.com/anuy-lang/anuy/experimental/parser"
	"github.com/anuy-lang/anuy/internal/semantic"
)

type Result struct{ Diagnostics []semantic.Diagnostic }

// AnalyzeSource parses the experimental program and translates it into kernel
// CFGs. An if statement emits its condition reads in the branching block,
// each branch is emitted into its own child scope, and a dedicated join
// block receives edges from every continuing path, so initialization is
// proven on all of them (RFC-001 §141.4). A loop emits condition reads in
// its header, wires a body block over a child scope with a back edge to the
// header, and an exit join intersecting the zero-iteration and body paths
// (RFC-003 §70–75, §170.28–29). A closure literal is analyzed as
// its own CFG seeded with the facts at the creation point (RFC-001 §48,
// RFC-003 §82); closure operations never update caller facts
// (RFC-003 §84, §170.31).
func AnalyzeSource(source string) (Result, error) {
	program, err := parser.Parse(source)
	if err != nil {
		return Result{}, err
	}
	b := newBuilder()
	b.emit(program.Statements, semantic.NewScope())
	result := Result{}
	result.Diagnostics = append(result.Diagnostics, b.diagnostics...)
	result.Diagnostics = append(result.Diagnostics, b.analyze().Diagnostics...)
	result.Diagnostics = append(result.Diagnostics, b.uncheckedErrors()...)
	return result, nil
}

// uncheckedErrors implements the R1 lint (CONTRACTS §3): a declared
// `error?` binding read nowhere yields exactly one Warning at its
// declaration span. Any read anywhere lifts it, assignments do not
// re-arm it (errcheck semantics), `_`-discards create no binding at all
// (GB-3), and nothing outside declared `error?` is checked. The order
// follows the declaration order (binding ids grow monotonically).
func (b *builder) uncheckedErrors() []semantic.Diagnostic {
	ids := make([]semantic.BindingID, 0, len(b.errSpans))
	for id := range b.errSpans {
		if !b.reads[id] {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	out := make([]semantic.Diagnostic, 0, len(ids))
	for _, id := range ids {
		span := b.errSpans[id]
		out = append(out, semantic.NewDiagnostic(semantic.UncheckedErrorDescriptor, id, semantic.SourceSpan{Start: span.Start, End: span.End}))
	}
	return out
}

type edge struct{ from, to semantic.BlockID }

// loopContext tracks the innermost open loop for `break`/`continue` wiring.
// exitFacts accumulates the intersection of facts on every edge entering the
// exit block: the zero-iteration path for condition/iteration loops plus each
// break point (RFC-003 §74). nil means no reachable path yet (an infinite
// loop without breaks).
type loopContext struct {
	header    semantic.BlockID
	exit      semantic.BlockID
	exitFacts map[semantic.BindingID]bool
	exitNN    map[semantic.BindingID]bool
}

// methodInfo is the flat-table entry of a declared method (story 08):
// result nullability feeds the call-shape classification, parameter
// nullabilities feed the D-3 argument check (story 18), purity opts the
// call out of both effects (3a+3b). Since story 35 the fallible shape
// feeds the D-2 check on try operands.
type methodInfo struct {
	hasResult      bool
	resultNullable bool
	pure           bool
	params         []semantic.Nullability
	fallible       bool
	successClasses []semantic.Nullability
	// Story 39 (RFC-004 §6.2.1): the declaring receiver form - the
	// normalized type name and whether the spelling was `*T`. The method
	// set of *T carries both forms, the set of T only value receivers.
	receiver string
	pointer  bool
}

// ifaceMethod is one interface method signature (story 39, RFC-004
// §6.1.1) reduced to the nullability-level match this layer compares.
type ifaceMethod struct {
	name           string
	params         []semantic.Nullability
	hasResult      bool
	resultNullable bool
	// resultError marks the `error?` result spelling (story 40): a
	// dispatch through the interface is fallible by its signature, not
	// by any concrete impl.
	resultError bool
}

// ifaceInfo is a declared interface (story 39): nominal identity plus
// the ordered method signatures.
type ifaceInfo struct {
	name    string
	methods []ifaceMethod
}

// declResult mirrors a declared function result type (story 08 Q4-A).
// Since story 35 (RFC-005 §6.2.3) it carries the strict fallible shape:
// the trailing `error?` flag and the per-success-result classes.
type declResult struct {
	hasResult      bool
	resultNullable bool
	fallible       bool
	successClasses []semantic.Nullability
}

type builder struct {
	blocks       []semantic.Block
	facts        []map[semantic.BindingID]bool // initialization facts at the end of each block
	nonNil       []map[semantic.BindingID]bool // non-nil narrowing facts, parallel to facts (story 05)
	pathNN       []map[string]bool             // non-nil narrowing facts of field paths, parallel to facts (story 27, ADR-0008)
	fallible     bool                          // the analyzed function body is fallible: declared result `error?` (story 34)
	successCount int                           // story 35 (RFC-005 §6.2.3): fallible success results; 0 for error-only and non-fallible bodies
	// correlated carries the conditional initialization of destructured
	// success results (story 36, RFC-005 §6.3): value binding -> its
	// controlling error binding. The entry exists until the value is
	// initialized outright (§6.3.4) or the guard is reassigned (§6.3.6);
	// the §6.3.5 FallibleResultID collapses to the guard binding id in
	// this layer.
	correlated map[semantic.BindingID]semantic.BindingID
	// terminated marks blocks whose flow does not continue to joined code
	// (story 36): a `return` block and the fresh block after it. emitIf
	// excludes terminated branch exits from the facts join - the surviving
	// path proves the branch condition (§6.3.2).
	terminated  []bool
	edges       []edge
	cur         int // index into blocks
	nextBlockID semantic.BlockID
	loops       []loopContext
	diagnostics []semantic.Diagnostic
	// nilable marks bindings declared with a nullable `T?` type - the only
	// nilability evidence in the type-free experimental layer.
	nilable map[semantic.BindingID]bool
	// classes carries the static nullability class per binding (story 08,
	// G1 decision): declared named types classify by their `?`, untyped
	// declarations infer from the RHS (§4). The class is set once at the
	// declaration site and never changes with the flow; unknown is the
	// zero value and keeps platform semantics.
	classes map[semantic.BindingID]semantic.Nullability
	// flowNullable records bindings whose latest assignment classified
	// null (`x = nil`, Q3-A of the story 08 types proposal). No v1
	// consumer - the seed for future flow-sensitive lints; consumers bring
	// their own join plumbing.
	flowNullable map[semantic.BindingID]bool
	// closureMutates registers, per closure binding, the captured bindings
	// its body assigns (RFC-003 §85 invalidation).
	closureMutates map[semantic.BindingID][]semantic.BindingID
	// declared/assigned feed the mutator computation for closure bodies.
	declared map[semantic.BindingID]bool
	assigned map[semantic.BindingID]bool
	// errSpans records the declaration span of every declared `error?`
	// binding (R1 lint, CONTRACTS §3); reads tracks every binding read
	// anywhere in the flow (flow-insensitive "read somewhere" is enough).
	errSpans map[semantic.BindingID]parser.Span
	reads    map[semantic.BindingID]bool
	// pure records declared functions annotated `//anuy:pure` (story 07
	// effects proposal): a trusted contract - their calls apply no
	// mutator set.
	pure map[semantic.BindingID]bool
	// methods is the flat method table (story 08 Q1-A): methods bind no
	// scope names and resolve by unique name; duplicates and collisions
	// with scope names reject.
	methods map[string]methodInfo
	// methodMutates registers, per method name, the captured bindings its
	// body assigns - the 3a half of the method-call effect.
	methodMutates map[string][]semantic.BindingID
	// funcParams carries the declared parameter classes per function name
	// (story 18, D-3); symmetric to funcResults, registered before the
	// body so a self-recursive call sees its own parameters.
	funcParams map[string][]semantic.Nullability
	// structs carries the declared struct table (story 22, RFC-014 6.2):
	// ordered field names and their classes - the completeness check uses
	// the names, the field-path classification the classes.
	structs map[string]*structInfo
	// enums carries the declared enum table (story 30, RFC-006 6.1):
	// enum name to ordered variant names - the variant-reference
	// classification (6.2) reads it.
	enums map[string][]string
	// bindingTypes carries the declared type name of a binding when it is
	// a named type (story 22): the root of a field path resolves through
	// it.
	bindingTypes map[semantic.BindingID]string
	// funcResults records declared function result types (story 08 Q4-A)
	// for the RHS call-shape classification (§26 establishment).
	funcResults map[string]declResult
	// interfaces carries the declared interface table (story 39, RFC-004
	// §6.1.1): name -> ordered method signatures.
	interfaces map[string]*ifaceInfo
	// impls records explicit conformances (story 39, RFC-004 §6.1.3):
	// interface -> target key (`Type` or `*Type`) -> declared. The
	// foundation of the slice-2 conversion checks.
	impls map[string]map[string]bool

	// unsafeDepth tracks the lexical unsafe context (story 41, RFC-007
	// §6.6.1-6.6.2): privileged operations require depth > 0. unsafeFuncs
	// records `unsafe func` declarations (§6.7.1) whose calls require the
	// context.
	unsafeDepth int
	unsafeFuncs map[string]bool
}

func newBuilder() *builder {
	return &builder{
		blocks:         []semantic.Block{{ID: 1}},
		facts:          []map[semantic.BindingID]bool{{}},
		nonNil:         []map[semantic.BindingID]bool{{}},
		pathNN:         []map[string]bool{{}},
		terminated:     []bool{false},
		nextBlockID:    1,
		correlated:     map[semantic.BindingID]semantic.BindingID{},
		nilable:        map[semantic.BindingID]bool{},
		classes:        map[semantic.BindingID]semantic.Nullability{},
		flowNullable:   map[semantic.BindingID]bool{},
		closureMutates: map[semantic.BindingID][]semantic.BindingID{},
		declared:       map[semantic.BindingID]bool{},
		assigned:       map[semantic.BindingID]bool{},
		errSpans:       map[semantic.BindingID]parser.Span{},
		reads:          map[semantic.BindingID]bool{},
		pure:           map[semantic.BindingID]bool{},
		methods:        map[string]methodInfo{},
		methodMutates:  map[string][]semantic.BindingID{},
		funcResults:    map[string]declResult{},
		funcParams:     map[string][]semantic.Nullability{},
		structs:        map[string]*structInfo{},
		enums:          map[string][]string{},
		bindingTypes:   map[semantic.BindingID]string{},
		interfaces:     map[string]*ifaceInfo{},
		impls:          map[string]map[string]bool{},
		unsafeFuncs:    map[string]bool{},
	}
}

func (b *builder) appendBlock() {
	b.nextBlockID++
	b.blocks = append(b.blocks, semantic.Block{ID: b.nextBlockID})
	b.facts = append(b.facts, map[semantic.BindingID]bool{})
	b.nonNil = append(b.nonNil, map[semantic.BindingID]bool{})
	b.pathNN = append(b.pathNN, map[string]bool{})
	b.terminated = append(b.terminated, false)
	b.cur = len(b.blocks) - 1
}

func (b *builder) add(op semantic.Operation) {
	switch op.Kind {
	case semantic.ReadOperation, semantic.DerefOperation:
		// Both operations read the binding's value: any of them lifts the
		// UncheckedError lint (CONTRACTS §3.3).
		b.reads[op.Binding] = true
	}
	b.blocks[b.cur].Operations = append(b.blocks[b.cur].Operations, op)
}

func (b *builder) declare(id semantic.BindingID) {
	b.add(semantic.Declare(id))
	b.facts[b.cur][id] = false
	b.nonNil[b.cur][id] = false
	b.declared[id] = true
}

func (b *builder) initialize(id semantic.BindingID) {
	b.add(semantic.Assign(id))
	b.facts[b.cur][id] = true
	// RFC-002 §25: assignment invalidates narrowing. Re-establishment (§26)
	// is the caller's move - the assignment path classifies the RHS and
	// emits the Assume after this invalidation (story 08).
	b.nonNil[b.cur][id] = false
	b.assigned[id] = true
	// Story 36 (RFC-005 §6.3.4/§6.3.6): an outright initialization
	// releases the value from its conditional state - the correlation
	// entry is stale from this point. A reassignment of the controlling
	// error kills the correlation of every still-conditional dependent
	// (§6.3.6): the entry turns dead (guard 0) so later `err == nil`
	// proofs cannot materialize it, while reads keep reporting D-4.
	// Already-materialized values keep their facts - they exist as
	// ordinary initialized values.
	delete(b.correlated, id)
	for valueID, guard := range b.correlated {
		if guard == id {
			b.correlated[valueID] = 0
		}
	}
}

// materializeCorrelations proves `guard == nil` on the current path
// (story 36, RFC-005 §6.3.2/§6.3.3): every still-conditional success
// result of this guard becomes definitely initialized in the current
// block's facts. Branch-local by construction - joins intersect the
// branch facts back to conditional.
func (b *builder) materializeCorrelations(guard semantic.BindingID) {
	for valueID, controlled := range b.correlated {
		if controlled == guard {
			b.facts[b.cur][valueID] = true
		}
	}
}

// condNilComparison resolves a bare `x != nil` / `x == nil` condition to
// the narrowed binding and its polarity (story 36): negated=true for the
// `!=` form. Zero means the condition carries no bare nil comparison -
// field-path forms stay with condNarrowPath/nilCheckOperand.
func (b *builder) condNilComparison(statement *parser.Statement, scope *semantic.Scope) (semantic.BindingID, bool) {
	for _, op := range []struct {
		text    string
		negated bool
	}{{"!=", true}, {"==", false}} {
		parts := strings.SplitN(statement.Cond, op.text, 2)
		if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" || strings.Contains(parts[0], ".") {
			continue
		}
		name := strings.TrimSpace(parts[0])
		for _, ident := range statement.CondIdents {
			if ident == name {
				return scope.Resolve(ident), op.negated
			}
		}
	}
	return 0, false
}

// assume establishes the non-nil narrowing fact for the lowering of a
// proven `x != nil` predicate (RFC-002 §22).
func (b *builder) assume(id semantic.BindingID) {
	b.add(semantic.Assume(id))
	b.nonNil[b.cur][id] = true
}

// needsNonNilProof reports whether an ordinary member access on the binding
// requires the non-nil proof: a declared `T?` (story 05 nilable set) or an
// inferred-nullable class (story 08 inference, Q2-A). Unknown classes keep
// the platform semantics - dereference free, nothing established.
func (b *builder) needsNonNilProof(id semantic.BindingID) bool {
	return b.nilable[id] || b.classes[id] == semantic.NullabilityNullable
}

// classifyValue assigns the nullability class of an RHS value per the
// accepted classification (story 08 types proposal §3): non-nil literals,
// declared non-null bindings, live non-nil facts and non-null-classified
// bindings are non-null; `nil` and unproven nullable bindings are null;
// calls (no signatures in this slice), navigation, composite raw text and
// closures are unknown and never establish (CONTRACTS §2.1).
func (b *builder) classifyValue(value *parser.Value, scope *semantic.Scope) semantic.Nullability {
	if value == nil || value.Closure != nil {
		return semantic.NullabilityUnknown
	}
	if value.Switch != nil {
		// Story 32 (RFC-006 §6.4.3): the switch value class is the
		// conservative join of its arm values - any unknown dominates,
		// then any nullable.
		join := semantic.NullabilityNonNull
		for _, arm := range value.Switch.Arms {
			if arm.Value == nil {
				return semantic.NullabilityUnknown
			}
			switch b.classifyValue(arm.Value, scope) {
			case semantic.NullabilityUnknown:
				return semantic.NullabilityUnknown
			case semantic.NullabilityNullable:
				join = semantic.NullabilityNullable
			}
		}
		return join
	}
	if value.Navigation != nil {
		// Story 30 (RFC-006 §6.1, §6.2): `Enum.Variant` is a variant
		// constant reference - non-null and establishing. An unknown
		// variant or enum falls through unknown (F-G3 tolerance).
		if len(value.Navigation.Segments) == 1 && !value.Navigation.Segments[0].Safe && !value.Navigation.Segments[0].Call {
			if variants, ok := b.enums[value.Navigation.Receiver]; ok {
				for _, variant := range variants {
					if variant == value.Navigation.Segments[0].Name {
						return semantic.NullabilityNonNull
					}
				}
			}
		}
		// Story 22/25: an ordinary field path carries the declared class
		// of its final field walked through declared struct types
		// (RFC-014 §6.7); anything else stays unknown.
		if fields, ok := pathFields(value.Navigation); ok {
			if id := scope.Resolve(value.Navigation.Receiver); id != 0 {
				if class := b.pathClass(id, fields); class != semantic.NullabilityUnknown {
					return class
				}
			}
		}
		return b.classifyNavigationValue(value.Navigation, scope)
	}
	if value.Text == "nil" {
		return semantic.NullabilityNullable
	}
	if len(value.Idents) == 1 && value.Text == value.Idents[0] {
		id := scope.Resolve(value.Idents[0])
		if id == 0 {
			return semantic.NullabilityUnknown
		}
		if b.nilable[id] {
			if b.nonNil[b.cur][id] {
				return semantic.NullabilityNonNull
			}
			return semantic.NullabilityNullable
		}
		return b.classes[id]
	}
	if len(value.Idents) == 0 && value.Text != "" {
		return semantic.NullabilityNonNull
	}
	if name, ok := assumeNonNullOperand(value); ok {
		// Story 41 (RFC-007 §6.6.5/§6.6.11): the intrinsic asserts the
		// operand non-null; the fact attaches to the operand binding in
		// the unsafe context. Outside the context the statement-level
		// check reports D-1 - classification stays conservative here.
		if id := scope.Resolve(name); id != 0 && b.unsafeDepth > 0 {
			b.assume(id)
		}
		return semantic.NullabilityNonNull
	}
	if strings.HasSuffix(value.Text, "()") {
		// A raw call shape (`name()`, `a.b()`): a declared result type
		// classifies the value (story 08 Q4-A); unresolved and void calls
		// stay unknown (CONTRACTS §2.1).
		return b.classifyCallShape(strings.TrimSuffix(value.Text, "()"))
	}
	return semantic.NullabilityUnknown
}

// assumeNonNullOperand reports the operand identifier of an
// `assume_non_nil(x)` value shape (story 41): the parser renders the
// intrinsic call as raw text plus the callee and operand idents.
func assumeNonNullOperand(value *parser.Value) (string, bool) {
	if value == nil || len(value.Idents) != 2 || value.Idents[0] != "assume_non_nil" {
		return "", false
	}
	return value.Idents[1], true
}

// methodKey builds the per-type method-set key (story 39, RFC-004
// §6.1.5): the normalized receiver type (pointer spelling stripped) and
// the method name.
func methodKey(receiver, name string) string {
	return strings.TrimPrefix(receiver, "*") + "." + name
}

// resolveMethod resolves a method by name with the receiver's declared
// type (story 39): bindingTypes give the `Type.name` key; an unknown
// receiver falls back to the name-only match when exactly one method
// carries the name. Ambiguity and absence stay in the tolerance zone.
// Story 40 (RFC-004 §8.1.5): an interface-typed receiver resolves
// through the exact interface method set - the signature (result class,
// fallibility, parameter classes) comes from the interface, and a
// member outside the set never falls back to the name-only match.
func (b *builder) resolveMethod(name string, receiver semantic.BindingID) (string, methodInfo, bool) {
	if receiver != 0 {
		if typeName := b.bindingTypes[receiver]; typeName != "" {
			if iface, ok := b.interfaces[typeName]; ok {
				for _, sig := range iface.methods {
					if sig.name == name {
						return "", ifaceMethodInfo(sig), true
					}
				}
				return "", methodInfo{}, false
			}
			key := methodKey(typeName, name)
			if info, ok := b.methods[key]; ok {
				return key, info, true
			}
		}
	}
	var foundKey string
	var found methodInfo
	count := 0
	for key, info := range b.methods {
		if strings.HasSuffix(key, "."+name) {
			foundKey, found = key, info
			count++
			if count > 1 {
				return "", methodInfo{}, false
			}
		}
	}
	if count == 1 {
		return foundKey, found, true
	}
	return "", methodInfo{}, false
}

// ifaceMethodInfo lifts an interface signature into the method table
// shape (story 40): the result and parameter classes come from the
// interface declaration, fallibility from the `error?` spelling. Purity
// stays false - impl-specific behavior is unknown, so dispatch keeps the
// conservative receiver invalidation.
func ifaceMethodInfo(sig ifaceMethod) methodInfo {
	return methodInfo{
		hasResult:      sig.hasResult,
		resultNullable: sig.resultNullable,
		fallible:       sig.hasResult && sig.resultError && sig.resultNullable,
		params:         sig.params,
	}
}

// classifyNavigationValue classifies a selector-chain value: a plain
// member read has no model (no fields, §28) and stays unknown; a resolved
// method call carries its declared result (story 08 Q4-A), lifted to
// nullable through safe navigation (RFC-002 §38).
func (b *builder) classifyNavigationValue(nav *parser.NavigationExpr, scope *semantic.Scope) semantic.Nullability {
	last := nav.Segments[len(nav.Segments)-1]
	if !last.Call {
		return semantic.NullabilityUnknown
	}
	_, info, ok := b.resolveMethod(last.Name, scope.Resolve(nav.Receiver))
	if !ok || !info.hasResult {
		return semantic.NullabilityUnknown
	}
	if last.Safe || info.resultNullable {
		return semantic.NullabilityNullable
	}
	return semantic.NullabilityNonNull
}

// classifyCallShape resolves a raw `name()` / `a.b()` RHS call shape
// against the declared functions and the flat method table (story 08
// Q4-A): a known non-null result classifies non-null, a nullable result -
// null; void and unresolved shapes stay unknown and never establish.
func (b *builder) classifyCallShape(prefix string) semantic.Nullability {
	if i := strings.LastIndex(prefix, "."); i >= 0 {
		// Story 39: the prefix carries the receiver type (`File.Read`) -
		// the per-type key resolves directly, the name-only fallback
		// covers unknown receivers.
		if info, ok := b.methods[methodKey(prefix[:i], prefix[i+1:])]; ok {
			return classifyDeclResult(info.hasResult, info.resultNullable)
		}
		if _, info, ok := b.resolveMethod(prefix[i+1:], 0); ok {
			return classifyDeclResult(info.hasResult, info.resultNullable)
		}
		return semantic.NullabilityUnknown
	}
	if res, ok := b.funcResults[prefix]; ok {
		return classifyDeclResult(res.hasResult, res.resultNullable)
	}
	return semantic.NullabilityUnknown
}

func classifyDeclResult(hasResult, resultNullable bool) semantic.Nullability {
	if !hasResult {
		return semantic.NullabilityUnknown
	}
	if resultNullable {
		return semantic.NullabilityNullable
	}
	return semantic.NullabilityNonNull
}

// establish applies §26 after the §25 invalidation of an assignment: a
// non-null-classified RHS re-establishes the narrowing (the Assume after
// the Assign), a null RHS records the flow-nullable fact (Q3-A, no
// consumers in v1), unknown changes nothing. Story 20: a classified-null
// value on a declared non-null binding is the D-1 violation (RFC-002
// §6.1.7/§8.2.1) - reported at the assignment's span.
func (b *builder) establish(id semantic.BindingID, value *parser.Value, scope *semantic.Scope, span parser.Span) {
	switch b.classifyValue(value, scope) {
	case semantic.NullabilityNonNull:
		b.assume(id)
	case semantic.NullabilityNullable:
		b.flowNullable[id] = true
		if b.classes[id] == semantic.NullabilityNonNull {
			b.report(semantic.NilToNonNull, span)
		}
	}
}

// establishAssignments pairs assignment targets with their RHS values by
// index and applies §26 (story 08). A name/value count mismatch leaves the
// extra targets unestablished - the tuple evaluation order is not modeled.
func (b *builder) establishAssignments(statement *parser.Statement, scope *semantic.Scope) {
	for i := range statement.Values {
		if i >= len(statement.Names) {
			return
		}
		name := statement.Names[i]
		if name == "_" {
			continue
		}
		// Story 27 (ADR-0008): the assignment kills the narrowing facts of
		// the target and its extensions.
		b.killPathFacts(name)
		if id := scope.Resolve(name); id != 0 {
			b.establish(id, &statement.Values[i], scope, statement.Span)
		}
	}
}

func (b *builder) isNonNil(id semantic.BindingID) bool {
	return b.nonNil[b.cur][id]
}

// knownNonNull reports whether the binding is known to be non-null at the
// current point: a live narrowing fact (RFC-002 §6.3) or a declared
// non-null class (story 08 G1). Unknown classes stay unknown - the
// conservative side keeps them D-4 free.
func (b *builder) knownNonNull(id semantic.BindingID) bool {
	return b.isNonNil(id) || b.classes[id] == semantic.NullabilityNonNull
}

// structInfo carries the ordered fields of a declared struct (RFC-014
// 6.2): the names drive the completeness check (6.3), the classes and
// declared type names the field-path classification (6.7); the embedded
// flags mark the §6.9 `embed` fields - the promotion roots (6.10).
type structInfo struct {
	names    []string
	classes  []semantic.Nullability
	types    []string
	embedded []bool
}

// registerStruct adds a declared struct to the table (story 22); story 25
// records each field's declared type name so the path walk can continue
// through named struct fields (empty for every other shape).
// registerInterface records a declared interface (story 39, RFC-004
// §6.1.1): nominal identity plus ordered method signatures reduced to the
// nullability-level match. A duplicate interface name rejects.
func (b *builder) registerInterface(decl *parser.InterfaceDecl) {
	if _, exists := b.interfaces[decl.Name]; exists {
		b.report(semantic.SameScopeRedeclaration, decl.Span)
		return
	}
	info := &ifaceInfo{name: decl.Name}
	for _, method := range decl.Methods {
		sig := ifaceMethod{name: method.Name, hasResult: method.HasResult, resultNullable: method.ResultNullable}
		if method.ResultTypeExpr != nil && method.ResultTypeExpr.Kind == parser.NamedType && method.ResultTypeExpr.Name == "error" {
			sig.resultError = true
		}
		for _, p := range method.Params {
			sig.params = append(sig.params, nullabilityOfParam(p))
		}
		info.methods = append(info.methods, sig)
	}
	b.interfaces[decl.Name] = info
}

// emitImpl validates one explicit conformance (story 39, RFC-004
// §6.1.3): every interface method must exist in the target's effective
// method set (§6.2.1 - *T carries value and pointer receivers, T only
// value receivers) with a matching nullability-level signature. The
// conformance record feeds the slice-2 conversion checks; unknown
// interfaces report and stop.
func (b *builder) emitImpl(decl *parser.ImplDecl) {
	iface, ok := b.interfaces[decl.Interface]
	if !ok {
		b.report(semantic.ImplUnknownInterface, decl.Span)
		return
	}
	target := decl.Type
	if decl.Pointer {
		target = "*" + target
	}
	if b.impls[decl.Interface] == nil {
		b.impls[decl.Interface] = map[string]bool{}
	}
	if b.impls[decl.Interface][target] {
		// §6.5.1 coherence: one impl per (interface, target type).
		b.report(semantic.DuplicateImpl, decl.Span)
		return
	}
	b.impls[decl.Interface][target] = true
	for _, method := range iface.methods {
		key := methodKey(decl.Type, method.name)
		info, found := b.methods[key]
		if !found || (decl.Pointer == false && info.pointer) {
			// §6.2.1: a value-type method set carries only value
			// receivers; the pointer-type set carries both forms.
			b.report(semantic.InterfaceMethodMissing, decl.Span)
			continue
		}
		if !ifaceSignatureMatch(method, info) {
			b.report(semantic.InterfaceMethodMismatch, decl.Span)
		}
	}
}

// ifaceSignatureMatch compares an interface signature against a method at
// the nullability level (story 39): parameter count, per-parameter
// classes and the result class. Full type identity stays outside this
// slice - the experimental layer keeps types as raw text.
func ifaceSignatureMatch(method ifaceMethod, info methodInfo) bool {
	if len(method.params) != len(info.params) {
		return false
	}
	for i := range method.params {
		if method.params[i] != info.params[i] {
			return false
		}
	}
	return method.hasResult == info.hasResult && method.resultNullable == info.resultNullable
}

func (b *builder) registerStruct(sd *parser.StructDecl) {
	info := &structInfo{}
	for _, field := range sd.Fields {
		info.names = append(info.names, field.Name)
		info.classes = append(info.classes, classOfTypeExpr(field.TypeExpr))
		typeName := ""
		if field.TypeExpr != nil && field.TypeExpr.Kind == parser.NamedType {
			typeName = field.TypeExpr.Name
		}
		info.types = append(info.types, typeName)
		info.embedded = append(info.embedded, field.Embedded)
	}
	b.structs[sd.Name] = info
}

// registerEnum adds a declared enum to the table (story 30, RFC-006
// §6.1): the ordered variant names drive the variant-reference
// classification (§6.2).
func (b *builder) registerEnum(ed *parser.EnumDecl) {
	names := make([]string, 0, len(ed.Variants))
	for _, variant := range ed.Variants {
		names = append(names, variant.Name)
	}
	b.enums[ed.Name] = names
}

// classOfTypeExpr classifies a restricted type expression (story 22):
// named types carry their `?`; composite shapes stay Unknown (ADR-0002).
func classOfTypeExpr(t *parser.TypeExpr) semantic.Nullability {
	if t == nil {
		return semantic.NullabilityUnknown
	}
	if t.Kind == parser.NamedType {
		if t.Nullable {
			return semantic.NullabilityNullable
		}
		return semantic.NullabilityNonNull
	}
	return semantic.NullabilityUnknown
}

// fieldClass resolves the declared class of a one-level field path
// `root.field` (RFC-014 6.7).
func (b *builder) fieldClass(root semantic.BindingID, field string) semantic.Nullability {
	return b.pathClass(root, []string{field})
}

// pathClass resolves the declared class of an ordinary field path
// `root.f1.…fn` (story 25, RFC-014 §6.7): each intermediate field must be
// known non-null by declared class and carry a named struct type to walk
// through; the verdict is the final field's declared class. Nullable,
// pointer and composite intermediates stay unknown - the deref safety of
// the path is a separate slice (§6.7 narrowing); cycles cannot occur (a
// finite token path, §6.11 pointer indirection is not a named struct).
func (b *builder) pathClass(root semantic.BindingID, fields []string) semantic.Nullability {
	info, ok := b.rootStruct(root)
	if !ok {
		return semantic.NullabilityUnknown
	}
	for i, field := range fields {
		entry, ok := b.lookupField(info, field)
		if !ok {
			return semantic.NullabilityUnknown
		}
		if i == len(fields)-1 {
			return entry.class
		}
		if entry.class != semantic.NullabilityNonNull || entry.typeName == "" {
			return semantic.NullabilityUnknown
		}
		info, ok = b.structs[entry.typeName]
		if !ok {
			return semantic.NullabilityUnknown
		}
	}
	return semantic.NullabilityUnknown
}

// fieldEntry is one resolved field: declared class and declared type name
// (empty for every composite shape).
type fieldEntry struct {
	class    semantic.Nullability
	typeName string
}

// lookupField resolves a field name inside a struct (story 29, RFC-014
// §6.10): direct fields first, then promoted through embedded structs,
// breadth-first - the shallowest depth wins and two candidates on the
// same minimal depth are ambiguous, resolving unknown; the visited set
// guards embedding cycles (the §6.11 layout check is a follow-up).
func (b *builder) lookupField(info *structInfo, name string) (fieldEntry, bool) {
	for i, n := range info.names {
		if n == name {
			return fieldEntry{info.classes[i], info.types[i]}, true
		}
	}
	visited := map[string]bool{}
	current := []*structInfo{}
	for i := range info.names {
		if !info.embedded[i] || info.types[i] == "" || visited[info.types[i]] {
			continue
		}
		if sub, ok := b.structs[info.types[i]]; ok {
			visited[info.types[i]] = true
			current = append(current, sub)
		}
	}
	for len(current) > 0 {
		matches := 0
		var result fieldEntry
		next := []*structInfo{}
		for _, sub := range current {
			for i, n := range sub.names {
				if n == name {
					matches++
					result = fieldEntry{sub.classes[i], sub.types[i]}
				}
			}
		}
		if matches > 1 {
			// ambiguous on the minimal depth: unknown, but resolved
			return fieldEntry{}, true
		}
		if matches == 1 {
			return result, true
		}
		for _, sub := range current {
			for i := range sub.names {
				if !sub.embedded[i] || sub.types[i] == "" || visited[sub.types[i]] {
					continue
				}
				if deeper, ok := b.structs[sub.types[i]]; ok {
					visited[sub.types[i]] = true
					next = append(next, deeper)
				}
			}
		}
		current = next
	}
	return fieldEntry{}, false
}

// rootStruct resolves a binding's declared struct table entry.
func (b *builder) rootStruct(root semantic.BindingID) (*structInfo, bool) {
	typeName, ok := b.bindingTypes[root]
	if !ok {
		return nil, false
	}
	info, ok := b.structs[typeName]
	return info, ok
}

// pathKnownNonNull reports whether an ordinary field path is known
// non-null by declared classes only (6.3.7: no flow facts on fields).
// The root's own nullability does not participate - the final field's
// declared class makes the check verdict static (deref safety of the
// path is a separate follow-up).
func (b *builder) pathKnownNonNull(root semantic.BindingID, fields []string) bool {
	return b.pathClass(root, fields) == semantic.NullabilityNonNull
}

// pathFields reports the field names of a navigation whose segments are
// all ordinary (no safe, no call) - the shape the path walk classifies.
func pathFields(nav *parser.NavigationExpr) ([]string, bool) {
	fields := make([]string, 0, len(nav.Segments))
	for _, segment := range nav.Segments {
		if segment.Safe || segment.Call {
			return nil, false
		}
		fields = append(fields, segment.Name)
	}
	return fields, true
}

// safeSegmentIndex reports the index of the first safe segment in the
// chain, -1 when every segment is ordinary.
func safeSegmentIndex(nav *parser.NavigationExpr) int {
	for i, segment := range nav.Segments {
		if segment.Safe {
			return i
		}
	}
	return -1
}

// pathFieldsPrefix reports the field names of the ordinary segment prefix
// before index k - the base path behind a safe segment.
func pathFieldsPrefix(nav *parser.NavigationExpr, k int) ([]string, bool) {
	fields := make([]string, 0, k)
	for _, segment := range nav.Segments[:k] {
		if segment.Safe || segment.Call {
			return nil, false
		}
		fields = append(fields, segment.Name)
	}
	return fields, true
}

// gatePathDerefs reports UnsafeMemberAccess (ANUY4001) at the first
// ordinary segment traversed through a provably nullable prefix (story
// 26, RFC-014 §6.7: ordinary `.` MUST NOT resolve without narrowing) and
// returns true. Unknown prefixes stay silent - conservative; the safe
// form is the §6.7 way out and never gated. The root link (segment 0)
// stays on the Deref/CFG machine, so a true return suppresses the root
// Deref: one error per path, at the violating segment.
func (b *builder) gatePathDerefs(root semantic.BindingID, nav *parser.NavigationExpr) bool {
	for i := 1; i < len(nav.Segments); i++ {
		if nav.Segments[i].Safe || nav.Segments[i].Call {
			return false
		}
		fields, ok := pathFieldsPrefix(nav, i)
		if !ok {
			return false
		}
		if b.pathNN[b.cur][pathFactKey(nav.Receiver, fields)] {
			// Story 27: the `path != nil` narrowing proves this link.
			continue
		}
		switch b.pathClass(root, fields) {
		case semantic.NullabilityNullable:
			b.report(semantic.UnsafeMemberAccess, nav.Segments[i].Span)
			return true
		case semantic.NullabilityNonNull:
			// keep walking to the next link
		default:
			return false
		}
	}
	return false
}

// gatePathDerefsFields is the fields-only variant for positions without
// segment spans (the exact `X == nil` condition form): the report lands
// on the given statement span. receiver names the fact-key base.
func (b *builder) gatePathDerefsFields(root semantic.BindingID, receiver string, fields []string, span parser.Span) bool {
	for k := 1; k < len(fields); k++ {
		if b.pathNN[b.cur][pathFactKey(receiver, fields[:k])] {
			// Story 27: the `path != nil` narrowing proves this link.
			continue
		}
		switch b.pathClass(root, fields[:k]) {
		case semantic.NullabilityNullable:
			b.report(semantic.UnsafeMemberAccess, span)
			return true
		case semantic.NullabilityNonNull:
			// keep walking to the next link
		default:
			return false
		}
	}
	return false
}

// checkConstruction verifies the completeness of a keyed construction
// against the declared struct (RFC-014 6.3, story 22): every direct field
// present exactly once and every key resolving to a direct field. Unknown
// types stay conservatively unchecked; duplicate keys already reject at
// parse time (story 21).
func (b *builder) checkConstruction(keyed *parser.KeyedLiteral) {
	info, ok := b.structs[keyed.Name]
	if !ok {
		return
	}
	declared := map[string]bool{}
	for _, name := range info.names {
		declared[name] = true
	}
	present := map[string]bool{}
	for _, key := range keyed.Fields {
		if !declared[key] {
			b.report(semantic.IncompleteConstruction, keyed.Span)
			return
		}
		present[key] = true
	}
	for _, name := range info.names {
		if !present[name] {
			b.report(semantic.IncompleteConstruction, keyed.Span)
			return
		}
	}
}

func (b *builder) isInitialized(id semantic.BindingID) bool {
	return b.facts[b.cur][id]
}

func (b *builder) connect(from, to semantic.BlockID) {
	b.edges = append(b.edges, edge{from: from, to: to})
}

// analyze runs the kernel analyzer over the built CFG.
func (b *builder) analyze() semantic.AnalysisResult {
	return (semantic.Analyzer{}).Analyze(b.cfg())
}

// cfg materializes the kernel CFG of the blocks and edges collected so far.
func (b *builder) cfg() *semantic.CFG {
	cfg := semantic.NewCFG(b.blocks...)
	for _, e := range b.edges {
		cfg.AddEdge(e.from, e.to)
	}
	return cfg
}

func (b *builder) emit(statements []parser.Statement, scope *semantic.Scope) {
	for i := range statements {
		b.emitStatement(&statements[i], scope)
	}
}

func (b *builder) emitStatement(statement *parser.Statement, scope *semantic.Scope) {
	switch statement.Kind {
	case parser.TypeDecl:
		// Story 22/29: the struct table feeds the completeness check and
		// the field-path classification (RFC-014 §6.2/§6.7). Story 30:
		// enums register their variant table (RFC-006 §6.1).
		if statement.Enum != nil {
			b.registerEnum(statement.Enum)
		} else {
			b.registerStruct(statement.Struct)
		}
	case parser.Interface:
		// Story 39 (RFC-004 §6.1.1): the nominal interface table.
		b.registerInterface(statement.Interface)
	case parser.Impl:
		// Story 39 (RFC-004 §6.1.3): conformance validation at the
		// declaration point; the record feeds slice-2 conversions.
		b.emitImpl(statement.Impl)
	case parser.Switch:
		b.emitSwitch(statement, scope)
	case parser.Try:
		// Story 34 (RFC-005 §6.5.4): try requires a fallible enclosing
		// function; the call effects apply as for any call. Story 35
		// (§6.5.1, D-2): the operand call must carry the trailing `error?`.
		if !b.fallible {
			b.report(semantic.PropagationOutsideFallible, statement.Span)
		}
		if res, ok := b.calleeResults(statement.Call, scope); ok && !res.fallible {
			b.report(semantic.InvalidTry, statement.Span)
		}
		b.emitCall(statement, scope)
	case parser.Var:
		if statement.TryCall != nil {
			b.emitTryDecl(statement, scope)
			return
		}
		if statement.TypeExpr == nil && len(statement.Values) == 1 && callCalleePrefix(statement.Values[0].Text) != "" {
			// Story 36 (RFC-005 §6.3, §6.9.1): a fallible-call
			// destructuring `var data, err = Load()` creates conditional
			// success bindings; any other shape falls through to the
			// generic path.
			if b.emitVarDestructuring(statement, scope) {
				return
			}
		}
		b.readIdents(statement, scope)
		b.checkUnsafeCallValues(statement.Values, statement.Span)
		declared := make([]string, 0, len(statement.Names))
		targets := make([]semantic.BindingID, 0, len(statement.Names))
		for i, name := range statement.Names {
			if name == "_" {
				// GB-3 variant A: the blank identifier receives a value but
				// creates no binding (RFC-003 §65).
				continue
			}
			// Inference (§4, story 08) reads the RHS class before the
			// declaration enters its own scope - matching readIdents, which
			// resolves initializer idents before the declaration (RFC-003 §36).
			var inferred semantic.Nullability
			enumName := ""
			if statement.TypeExpr == nil && i < len(statement.Values) {
				inferred = b.classifyValue(&statement.Values[i], scope)
				// Story 30: a variant reference pins the binding's enum
				// type - the switch scrutinee resolves through it.
				if nav := statement.Values[i].Navigation; nav != nil && len(nav.Segments) == 1 && !nav.Segments[0].Safe && !nav.Segments[0].Call {
					if _, isEnum := b.enums[nav.Receiver]; isEnum {
						enumName = nav.Receiver
					}
				}
			}
			if serr := scope.Declare(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			declared = append(declared, name)
			id := scope.Resolve(name)
			targets = append(targets, id)
			b.declare(id)
			if enumName != "" {
				b.bindingTypes[id] = enumName
			}
			if statement.TypeExpr != nil {
				if statement.TypeExpr.Nullable {
					b.nilable[id] = true
					if statement.TypeExpr.Name == "error" {
						b.errSpans[id] = statement.Span
					}
				}
				// Declared class (story 08, G1): a named type is non-null or
				// nullable by its `?`; composite spellings stay unknown -
				// their nullability binding is outside the slice.
				if statement.TypeExpr.Kind == parser.NamedType {
					if statement.TypeExpr.Nullable {
						b.classes[id] = semantic.NullabilityNullable
					} else {
						b.classes[id] = semantic.NullabilityNonNull
					}
					// Story 22: the named type roots field-path classification.
					b.bindingTypes[id] = statement.TypeExpr.Name
				} else if statement.TypeExpr.Kind == parser.PointerType && statement.TypeExpr.Elem != nil && statement.TypeExpr.Elem.Kind == parser.NamedType {
					// Story 40: the `*T` spelling names the binding's type
					// for conversion checks and per-type method resolution.
					// Classes stay untouched (bare `*T` remains nil-tolerant,
					// RFC-002 §6.2.3) and field paths keep the tolerance -
					// rootStruct matches the struct table by bare name.
					b.bindingTypes[id] = "*" + statement.TypeExpr.Elem.Name
				}
			} else {
				b.classes[id] = inferred
				// Story 22: a keyed construction names its type - the field
				// model uses it for path classification (regardless of the
				// inferred class).
				if i < len(statement.Values) && statement.Values[i].Keyed != nil {
					b.bindingTypes[id] = statement.Values[i].Keyed.Name
				}
			}
		}
		if statement.Values != nil {
			for _, name := range declared {
				b.initialize(scope.Resolve(name))
			}
			// ADR-0001 (RFC-003 §13.8: first initialization uses ordinary
			// `=`): the initializer classifies like an assignment RHS, so a
			// non-null value re-establishes after the §25 invalidation of
			// initialize (§6.3.5).
			b.establishAssignments(statement, scope)
		}
		if statement.TypeExpr != nil {
			b.checkInterfaceConversions(statement, scope)
		}
		b.analyzeClosures(statement, scope, targets)
	case parser.Assign:
		if statement.Target != nil {
			// Story 22: field mutation — RHS reads, then the D-1 check on the
			// field path (RFC-014 §6.7: mutation respects the declared field
			// type). Fields carry no flow facts (§6.3.7).
			b.readIdents(statement, scope)
			root := scope.Resolve(statement.Target.Receiver)
			fields := make([]string, 0, len(statement.Target.Segments))
			for _, segment := range statement.Target.Segments {
				fields = append(fields, segment.Name)
			}
			if root != 0 && !b.gatePathDerefs(root, statement.Target) && b.pathKnownNonNull(root, fields) &&
				b.classifyValue(&statement.Values[0], scope) == semantic.NullabilityNullable {
				b.report(semantic.NilToNonNull, statement.Span)
			}
			b.killPathFacts(pathFactKey(statement.Target.Receiver, fields))
			b.analyzeClosures(statement, scope, nil)
			return
		}
		b.readIdents(statement, scope)
		b.checkUnsafeCallValues(statement.Values, statement.Span)
		targets := make([]semantic.BindingID, 0, len(statement.Names))
		for _, name := range statement.Names {
			if name == "_" {
				// GB-3 variant A: a blank target receives the value without
				// creating or initializing any binding.
				continue
			}
			if serr := scope.Assign(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			targets = append(targets, scope.Resolve(name))
			b.initialize(scope.Resolve(name))
		}
		b.establishAssignments(statement, scope)
		b.analyzeClosures(statement, scope, targets)
	case parser.Read:
		b.readIdent(statement.Names[0], scope, statement.Span, nil)
	case parser.Call:
		b.emitCall(statement, scope)
		// Story 27 (ADR-0008): any call kills all path facts - the args
		// were read with the facts alive, the call effect invalidates them
		// for the statements that follow. Story 28: a resolved
		// `//anuy:pure` callee cannot invalidate the receiver - its facts
		// survive (ADR-0008 follow-up).
		if !b.callPreservesPathFacts(statement, scope) {
			b.killAllPathFacts()
		}
	case parser.Function:
		b.emitFunction(statement, scope)
	case parser.Return:
		b.emitReturn(statement, scope)
	case parser.If:
		b.emitIf(statement, scope)
	case parser.Loop:
		b.killAllPathFacts()
		b.emitLoop(statement, scope)
	case parser.Break:
		b.emitJump(true)
	case parser.Continue:
		b.emitJump(false)
	case parser.Block:
		// RFC-003 §25: the block creates a child scope; locals declared
		// inside do not survive it. Linear flow — no join block required.
		b.emit(statement.Body, scope.Child())
	case parser.UnsafeBlock:
		// Story 41 (RFC-007 §6.6.1-6.6.3): a lexical context only - the
		// body analyzes with the depth raised, and ordinary checking
		// (initialization, must-consume, conformance) stays fully on.
		b.unsafeDepth++
		b.emit(statement.Body, scope.Child())
		b.unsafeDepth--
	}
}

// checkInterfaceConversions validates concrete-to-interface conversions
// at the declaration point (story 40, RFC-004 §6.4.1, §6.4.4, §8.1.4
// D-4): a non-null RHS whose concrete type is known must carry the impl
// record (story 39). Identity conversions are free (§6.4.4.1); unknown
// concrete types (calls, fields - no type names in the tables) and
// nullable values stay unchecked - boxing of nullable concrete values is
// delegated to RFC-002 §6.9.17 (OQ-1).
func (b *builder) checkInterfaceConversions(statement *parser.Statement, scope *semantic.Scope) {
	ifaceName := statement.TypeExpr.Name
	if statement.TypeExpr.Kind != parser.NamedType || b.interfaces[ifaceName] == nil {
		return
	}
	for i := range statement.Values {
		if i >= len(statement.Names) || statement.Names[i] == "_" {
			continue
		}
		rhs := &statement.Values[i]
		if b.classifyValue(rhs, scope) != semantic.NullabilityNonNull {
			continue
		}
		rhsType := b.concreteValueType(rhs, scope)
		if rhsType == "" || rhsType == ifaceName {
			continue
		}
		if !b.impls[ifaceName][rhsType] {
			b.report(semantic.MissingExplicitConformance, statement.Span)
			return
		}
	}
}

// concreteValueType names the concrete type of a conversion RHS when the
// layer can prove it: a typed binding (bindingTypes, including the `*T`
// spelling) or a keyed construction. Everything else - calls, field
// paths, closures - returns "" and marks the tolerance zone.
func (b *builder) concreteValueType(value *parser.Value, scope *semantic.Scope) string {
	if value == nil {
		return ""
	}
	if value.Keyed != nil {
		return value.Keyed.Name
	}
	if len(value.Idents) == 1 && value.Text == value.Idents[0] {
		return b.bindingTypes[scope.Resolve(value.Idents[0])]
	}
	return ""
}

// analyzeClosures analyzes closure literals of the statement at its creation
// point, after the left-hand side facts are recorded. targets are the
// binding ids the closure values are assigned to; the closure's mutated
// captures are registered there for RFC-003 §85 call invalidation.
func (b *builder) analyzeClosures(statement *parser.Statement, scope *semantic.Scope, targets []semantic.BindingID) {
	index := 0
	for _, value := range statement.Values {
		if value.Closure == nil {
			continue
		}
		mutators, _ := b.analyzeClosure(value.Closure, scope, false)
		if index < len(targets) {
			// Решение 1 (2026-09-16): union across all assignments of the
			// binding — a later closure must not erase an earlier mutator.
			b.closureMutates[targets[index]] = unionBindings(b.closureMutates[targets[index]], mutators)
		}
		index++
	}
	b.propagateMutators(statement, scope, targets)
}

// nilCheckOperand resolves the binding of the exact `X == nil` condition
// (the mirror of condNarrowTarget's `!=` form); zero means the condition
// carries no such shape.
func (b *builder) nilCheckOperand(statement *parser.Statement, scope *semantic.Scope) (semantic.BindingID, string, []string) {
	parts := strings.SplitN(statement.Cond, "==", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return 0, "", nil
	}
	lhs := strings.TrimSpace(parts[0])
	// Story 22/25: an ordinary field path `u.f` / `u.f.g` names its root
	// through the condition identifiers.
	if dot := strings.Index(lhs, "."); dot > 0 {
		root := strings.TrimSpace(lhs[:dot])
		fields := strings.Split(lhs[dot+1:], ".")
		for _, field := range fields {
			if field == "" || strings.ContainsAny(field, " \t") {
				return 0, "", nil
			}
		}
		for _, ident := range statement.CondIdents {
			if ident == root {
				return scope.Resolve(ident), root, fields
			}
		}
		return 0, "", nil
	}
	for _, ident := range statement.CondIdents {
		if ident == lhs {
			return scope.Resolve(ident), lhs, nil
		}
	}
	return 0, "", nil
}

// reportRedundantNilCheck reports D-5 (RFC-002 §6.2.4/§8.2.5, story 19):
// the exact `X == nil` condition over a binding known to be non-null
// (a live narrowing fact or a declared non-null class). Story 22 extends
// the shape to one-level field paths `u.f`, known by declared classes
// only (§6.3.7 - no flow facts on fields). The `!= nil` form stays
// outside the slice - it re-establishes narrowing in the kernel
// (condNarrowTarget).
func (b *builder) reportRedundantNilCheck(statement *parser.Statement, scope *semantic.Scope) {
	id, receiver, fields := b.nilCheckOperand(statement, scope)
	if id == 0 {
		return
	}
	if fields != nil {
		if !b.gatePathDerefsFields(id, receiver, fields, statement.Span) && b.pathKnownNonNull(id, fields) {
			b.report(semantic.RedundantNilCheck, statement.Span)
		}
		return
	}
	if b.knownNonNull(id) {
		b.report(semantic.RedundantNilCheck, statement.Span)
	}
}

func (b *builder) emitIf(statement *parser.Statement, scope *semantic.Scope) {
	// Condition reads evaluate in the branching block, before any branch.
	b.readConditionIdents(statement, scope)
	b.reportRedundantNilCheck(statement, scope)
	before := copyFacts(b.facts[b.cur])
	beforeNN := copyFacts(b.nonNil[b.cur])
	beforePath := copyPathFacts(b.pathNN[b.cur])
	start := b.blocks[b.cur].ID
	// Story 36 (RFC-005 §6.3): a bare nil comparison carries correlation
	// polarity - the `!=` branch is the failure side, the `==` branch and
	// the opposite path prove `guard == nil` and materialize the
	// conditional success results (§6.3.2/§6.3.3).
	guardID, negated := b.condNilComparison(statement, scope)
	b.appendBlock()
	thenID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(before)
	if !negated && guardID != 0 {
		// `x == nil` branch: the success side - conditional results of x
		// exist here.
		b.materializeCorrelations(guardID)
	}
	b.nonNil[b.cur] = copyFacts(beforeNN)
	b.pathNN[b.cur] = copyPathFacts(beforePath)
	if narrowID := b.condNarrowTarget(statement, scope); narrowID != 0 {
		b.assume(narrowID)
	}
	if p := b.condNarrowPath(statement, scope); p != "" {
		// Story 27 (ADR-0008): `u.link != nil` narrows the path in the
		// then-branch only.
		b.pathNN[b.cur][p] = true
	}
	b.emit(statement.Body, scope.Child())
	thenFacts := copyFacts(b.facts[b.cur])
	thenNN := copyFacts(b.nonNil[b.cur])
	thenPath := copyPathFacts(b.pathNN[b.cur])
	thenExit := b.blocks[b.cur].ID
	thenTerminated := b.terminated[b.cur]

	// The implicit (or explicit) else side of `x != nil` proves `x == nil`
	// - conditional results materialize there (§6.3.4: a plain assignment
	// in the `!=` branch joins with the proven success path).
	elseBase := before
	if negated && guardID != 0 {
		elseBase = copyFacts(before)
		b.facts[b.cur] = elseBase
		b.materializeCorrelations(guardID)
		elseBase = copyFacts(b.facts[b.cur])
	}

	hasElse := statement.Else != nil
	var elseID, elseExit semantic.BlockID
	var elseFacts, elseNN map[semantic.BindingID]bool
	var elsePath map[string]bool
	elseTerminated := false
	if hasElse {
		b.appendBlock()
		elseID = b.blocks[b.cur].ID
		b.facts[b.cur] = copyFacts(elseBase)
		b.nonNil[b.cur] = copyFacts(beforeNN)
		b.pathNN[b.cur] = copyPathFacts(beforePath)
		b.emit(statement.Else, scope.Child())
		elseFacts = copyFacts(b.facts[b.cur])
		elseNN = copyFacts(b.nonNil[b.cur])
		elsePath = copyPathFacts(b.pathNN[b.cur])
		elseExit = b.blocks[b.cur].ID
		elseTerminated = b.terminated[b.cur]
	} else {
		elseFacts = elseBase
		elseNN = beforeNN
		elsePath = beforePath
	}

	// Story 36 (§6.3.2): a terminated branch contributes no facts to the
	// join - the surviving path carries the proof (its correlation side
	// was materialized above). Both branches diverging leaves the join
	// unreachable (D-6 sees no fall-off); without an else the implicit
	// else path always survives.
	joinUnreachable := hasElse && thenTerminated && elseTerminated
	var joinFacts, joinNN map[semantic.BindingID]bool
	var joinPath map[string]bool
	switch {
	case thenTerminated && !elseTerminated:
		joinFacts, joinNN, joinPath = elseFacts, elseNN, elsePath
	case elseTerminated && !thenTerminated:
		joinFacts, joinNN, joinPath = thenFacts, thenNN, thenPath
	case thenTerminated && elseTerminated:
		joinFacts, joinNN, joinPath = elseFacts, elseNN, elsePath
	default:
		joinFacts = intersectFacts(thenFacts, elseFacts)
		joinNN = intersectFacts(thenNN, elseNN)
		joinPath = intersectPathFacts(thenPath, elsePath)
	}

	b.appendBlock()
	b.facts[b.cur] = joinFacts
	b.nonNil[b.cur] = joinNN
	b.pathNN[b.cur] = joinPath
	joinID := b.blocks[b.cur].ID
	b.connect(start, thenID)
	if !thenTerminated && !joinUnreachable {
		b.connect(thenExit, joinID)
	}
	if hasElse {
		b.connect(start, elseID)
		if !elseTerminated && !joinUnreachable {
			b.connect(elseExit, joinID)
		}
	} else {
		b.connect(start, joinID)
	}
}

// emitSwitch lowers the exhaustive enum switch into the CFG (story 31,
// RFC-006 §6.3): the scrutinee reads exactly once (§6.3.2), each arm runs
// in a child scope on copies of the entry facts, and the join intersects
// every arm exit - exhaustive means no implicit skip path (§6.3.3).
// Exhaustiveness and duplicates check against the declared enum
// (§6.3.4, §6.3.5); an off-enum pattern reports §6.1's closed set. An
// untyped scrutinee stays conservative (F-G3).
func (b *builder) emitSwitch(statement *parser.Statement, scope *semantic.Scope) {
	sw := statement.Switch
	id, variants, knownEnum := b.switchEnum(sw.Scrutinee, scope)
	if id != 0 {
		b.add(semantic.Read(id))
	}
	// Story 33 (RFC-006 §6.5.2): for a nullable enum the exhaustive set
	// is {nil} ∪ variants; a nil arm on a non-null enum is unreachable
	// (§6.5.3).
	nullable := knownEnum && b.classes[id] == semantic.NullabilityNullable
	if knownEnum {
		b.reportSwitchArmIssues(variants, sw.Arms, statement.Span, nullable)
	}
	beforeF := copyFacts(b.facts[b.cur])
	beforeNN := copyFacts(b.nonNil[b.cur])
	beforePath := copyPathFacts(b.pathNN[b.cur])
	start := b.blocks[b.cur].ID
	joinF, joinNN, joinPath := beforeF, beforeNN, beforePath
	first := true
	var armIDs, armExits []semantic.BlockID
	for _, arm := range sw.Arms {
		b.appendBlock()
		armIDs = append(armIDs, b.blocks[b.cur].ID)
		b.facts[b.cur] = copyFacts(beforeF)
		b.nonNil[b.cur] = copyFacts(beforeNN)
		b.pathNN[b.cur] = copyPathFacts(beforePath)
		if knownEnum && nullable && !arm.NilArm && id != 0 {
			// Story 33 (RFC-006 §6.5.4): a variant arm proves the
			// scrutinee non-nil - ordinary control-flow facts (§6.5.4).
			b.assume(id)
		}
		b.emit(arm.Body, scope.Child())
		armExits = append(armExits, b.blocks[b.cur].ID)
		armF := copyFacts(b.facts[b.cur])
		armNN := copyFacts(b.nonNil[b.cur])
		armPath := copyPathFacts(b.pathNN[b.cur])
		if first {
			joinF, joinNN, joinPath = armF, armNN, armPath
			first = false
			continue
		}
		joinF = intersectFacts(joinF, armF)
		joinNN = intersectFacts(joinNN, armNN)
		joinPath = intersectPathFacts(joinPath, armPath)
	}
	b.appendBlock()
	b.facts[b.cur] = joinF
	b.nonNil[b.cur] = joinNN
	b.pathNN[b.cur] = joinPath
	joinID := b.blocks[b.cur].ID
	for _, armID := range armIDs {
		b.connect(start, armID)
	}
	for _, armExit := range armExits {
		b.connect(armExit, joinID)
	}
}

// switchEnum resolves a switch scrutinee (story 31): the binding's
// declared type name must be a known enum.
func (b *builder) switchEnum(scrutinee string, scope *semantic.Scope) (semantic.BindingID, []string, bool) {
	id := scope.Resolve(scrutinee)
	if id == 0 {
		return 0, nil, false
	}
	variants, known := b.enums[b.bindingTypes[id]]
	return id, variants, known
}

// reportSwitchArmIssues reports the story 31 diagnostics for a switch
// against its declared variants (§6.3.4 missing, §6.3.5 duplicate, §6.1
// off-enum patterns); the missing-variant report lands on the switch
// span.
func (b *builder) reportSwitchArmIssues(variants []string, arms []parser.SwitchArm, span parser.Span, nullable bool) {
	covered := map[string]bool{}
	missing := false
	for _, arm := range arms {
		if arm.NilArm {
			// Story 33 (§6.5.3): nil is reachable only for a nullable
			// scrutinee.
			if !nullable {
				b.report(semantic.NilArmOnNonNullEnum, arm.Span)
			}
			covered["nil"] = true
			continue
		}
		found := false
		for _, variant := range variants {
			if variant == arm.Variant {
				found = true
				break
			}
		}
		if !found {
			b.report(semantic.UnknownMatchVariant, arm.Span)
			continue
		}
		if covered[arm.Variant] {
			b.report(semantic.DuplicateMatchArm, arm.Span)
		}
		covered[arm.Variant] = true
	}
	for _, variant := range variants {
		if !covered[variant] {
			missing = true
			break
		}
	}
	if nullable && !covered["nil"] {
		missing = true
	}
	if missing {
		b.report(semantic.MissingEnumVariant, span)
	}
}

// emitLoop wires a loop CFG (spec 1-3-1-1, RFC-003 §70–78): condition and
// collection reads evaluate in the header on every iteration, the body block
// starts from header facts so reads are proven per iteration (loop-carried),
// and the exit keeps a binding initialized only on paths that actually reach
// it — the zero-iteration path plus every break point (§170.28–29). The back
// edge body→header lets the analyzer's fixpoint downgrade facts that only
// the body establishes. An infinite loop (`for { }`) has no condition exit:
// its exit is reachable only through break edges and stays fully
// unreachable until one exists (no proofs required, RFC-001 §39).
func (b *builder) emitLoop(statement *parser.Statement, scope *semantic.Scope) {
	entryID := b.blocks[b.cur].ID
	b.appendBlock()
	headerID := b.blocks[b.cur].ID
	b.readConditionIdents(statement, scope)
	b.reportRedundantNilCheck(statement, scope)
	if len(statement.Values) > 0 {
		// The iteration collection evaluates in the header on every
		// iteration (RFC-003 §76). Collection resolution has no binding
		// model yet, so an unresolved collection stays invisible here.
		for _, ident := range statement.Values[0].Idents {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
		}
	}
	headerFacts := copyFacts(b.facts[b.cur])
	headerNN := copyFacts(b.nonNil[b.cur])

	b.appendBlock()
	exitIdx := b.cur
	exitID := b.blocks[exitIdx].ID
	// Only condition/iteration loops have a zero-iteration exit path; an
	// infinite loop's exit is reachable exclusively through breaks. The flag
	// is captured before the body because breaks replace the nil exitFacts.
	hasConditionExit := statement.Cond != "" || len(statement.Values) > 0
	var exitFacts map[semantic.BindingID]bool
	var exitNN map[semantic.BindingID]bool
	if hasConditionExit {
		exitFacts = copyFacts(headerFacts)
		exitNN = copyFacts(headerNN)
	}

	b.appendBlock()
	bodyID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(headerFacts)
	b.nonNil[b.cur] = copyFacts(headerNN)
	// A bare `x != nil` loop condition re-establishes the narrowing for the
	// body on every iteration; the exit path stays unproven.
	if narrowID := b.condNarrowTarget(statement, scope); narrowID != 0 {
		b.assume(narrowID)
	}
	bodyScope := scope.Child()
	if len(statement.Names) > 0 {
		// RFC-003 §76, §27: the binding lives in the loop's child scope and
		// is initialized for each logical iteration; it shadows outer names
		// (§78, §170.20) and is invisible after the loop. The static single
		// binding records §77/§170.32 per-iteration identity only as an
		// observation (integration tests), not as a mechanism.
		if serr := bodyScope.Declare(statement.Names[0]); serr != nil {
			b.report(serr.Category, statement.Span)
		} else {
			id := bodyScope.Resolve(statement.Names[0])
			b.declare(id)
			b.initialize(id)
		}
	}
	b.loops = append(b.loops, loopContext{header: headerID, exit: exitID, exitFacts: exitFacts, exitNN: exitNN})
	b.emit(statement.Body, bodyScope)
	loopCtx := &b.loops[len(b.loops)-1]
	b.connect(b.blocks[b.cur].ID, headerID)
	b.loops = b.loops[:len(b.loops)-1]

	b.facts[exitIdx] = loopCtx.exitFacts
	b.nonNil[exitIdx] = loopCtx.exitNN
	b.cur = exitIdx // post-loop statements emit in the exit/join block

	b.connect(entryID, headerID)
	b.connect(headerID, bodyID)
	if hasConditionExit {
		b.connect(headerID, exitID)
	}
}

// emitJump wires `break` (edge to the loop exit) and `continue` (edge to the
// loop header), both targeting the nearest enclosing loop. The jump edge
// carries exactly the facts at the jump point: statements after the jump
// start an unreachable block, and every break point intersects its facts
// into the exit (RFC-003 §74 — only reachable exits contribute to
// post-loop state).
func (b *builder) emitJump(isBreak bool) {
	if len(b.loops) == 0 {
		return // unreachable: the parser rejects jumps outside loops
	}
	ctx := &b.loops[len(b.loops)-1]
	curFacts := copyFacts(b.facts[b.cur])
	curNN := copyFacts(b.nonNil[b.cur])
	if isBreak {
		b.connect(b.blocks[b.cur].ID, ctx.exit)
		if ctx.exitFacts == nil {
			ctx.exitFacts = curFacts
		} else {
			ctx.exitFacts = intersectFacts(ctx.exitFacts, curFacts)
		}
		if ctx.exitNN == nil {
			ctx.exitNN = curNN
		} else {
			ctx.exitNN = intersectFacts(ctx.exitNN, curNN)
		}
	} else {
		b.connect(b.blocks[b.cur].ID, ctx.header)
	}
	b.appendBlock()
	b.facts[b.cur] = curFacts
	b.nonNil[b.cur] = curNN
}

// readIdents emits reads for identifiers referenced by right-hand side
// expressions before any left-hand side operation, modeling "RHS values are
// evaluated before any LHS updates become observable" (RFC-003 §61).
// Declarations are not yet in scope for their own initializers (RFC-003 §36).
// Closure values carry no top-level idents; their bodies are analyzed as
// separate CFGs (RFC-003 §80–84). Unresolved RHS idents — including call
// callees, for which the grammar has no declaration form yet — stay
// invisible here: recorded conformance sources pin them as accepted.
func (b *builder) readIdents(statement *parser.Statement, scope *semantic.Scope) {
	unavailable := map[semantic.BindingID]bool{}
	for vi, value := range statement.Values {
		if value.Closure != nil {
			continue
		}
		// Story 41 (RFC-007 §6.6.13): the intrinsic callee is not a name
		// read - only the operand ident resolves.
		_, intrinsic := assumeNonNullOperand(&statement.Values[vi])
		for j, ident := range value.Idents {
			if intrinsic && j == 0 {
				continue
			}
			if id := scope.Resolve(ident); id != 0 {
				// Story 36 (RFC-005 §6.3/§8.1.4, D-4): a still-conditional
				// success result does not exist until the controlling
				// error is proven nil on this path - one report per
				// binding per statement.
				if _, conditional := b.correlated[id]; conditional && !b.facts[b.cur][id] && !unavailable[id] {
					unavailable[id] = true
					b.report(semantic.UnavailableSuccessResult, statement.Span)
				}
				b.add(semantic.Read(id))
			}
		}
		// Story 22: keyed constructions check their completeness against
		// the declared struct (RFC-014 §6.3).
		if value.Keyed != nil {
			b.checkConstruction(value.Keyed)
		}
		// Story 17: a receiver-safe segment on a known non-null receiver is
		// redundant (D-4, RFC-002 §8.2.4, ADR-0005). An ordinary member
		// access on a declared `T?` receiver requires the non-nil proof
		// (RFC-002 §22). Unresolved receivers stay in the F-G3 tolerance
		// zone.
		if value.Navigation != nil && len(value.Navigation.Segments) > 0 {
			if id := scope.Resolve(value.Navigation.Receiver); id != 0 {
				if value.Navigation.Segments[0].Safe {
					if b.knownNonNull(id) {
						b.report(semantic.RedundantSafeNavigation, value.Navigation.Segments[0].Span)
					}
				} else if !b.gatePathDerefs(id, value.Navigation) && b.needsNonNilProof(id) {
					b.add(semantic.Deref(id))
				}
				// Story 22/25: a safe segment behind an ordinary field path is
				// redundant when the prefix path is known non-null by declared
				// classes (6.3.7 - no flow facts on fields).
				if safeAt := safeSegmentIndex(value.Navigation); safeAt > 0 {
					if fields, ok := pathFieldsPrefix(value.Navigation, safeAt); ok && b.pathKnownNonNull(id, fields) {
						b.report(semantic.RedundantSafeNavigation, value.Navigation.Segments[safeAt].Span)
					}
				}
			}
		}
		// Story 32: a value-producing switch carries the same
		// exhaustiveness contract (§6.3.4) as the statement form.
		if value.Switch != nil {
			if id, variants, known := b.switchEnum(value.Switch.Scrutinee, scope); known {
				nullable := b.classes[id] == semantic.NullabilityNullable
				b.reportSwitchArmIssues(variants, value.Switch.Arms, value.Switch.Span, nullable)
			}
		}
	}
}

// readConditionIdents emits the condition reads of an if/loop header. A
// condition that is exactly one bare identifier is a binding read: unresolved,
// it reports one UnknownRead (D-01). Compound conditions keep calls and
// navigation opaque — the grammar has no declaration form for callees yet, so
// only their resolved idents become reads.
func (b *builder) readConditionIdents(statement *parser.Statement, scope *semantic.Scope) {
	reported := map[string]bool{}
	for _, ident := range statement.CondIdents {
		if statement.Cond != ident {
			if id := scope.Resolve(ident); id != 0 {
				b.add(semantic.Read(id))
			}
			continue
		}
		b.readIdent(ident, scope, statement.Span, reported)
	}
}

// readIdent emits the read of a name that resolves through the scope chain
// and reports exactly one UnknownRead per unresolved name at the read span
// (D-01, the RFC-003 §14 analogue on the read side). ReadBeforeInitialization
// stays reserved for resolved bindings lacking definite initialization, and
// the assignment path is untouched. reported dedupes repeated names within
// one statement; nil works for single-name sites.
func (b *builder) readIdent(name string, scope *semantic.Scope, span parser.Span, reported map[string]bool) {
	if serr := scope.Read(name); serr != nil {
		if !reported[name] {
			if reported != nil {
				reported[name] = true
			}
			b.report(serr.Category, span)
		}
		return
	}
	b.add(semantic.Read(scope.Resolve(name)))
}

// analyzeClosure models a closure body as its own CFG whose entry facts are
// the facts at the closure creation point:
//
//   - parameters are initialized at entry (RFC-003 §86);
//   - if the body reads a captured binding, that binding must be definitely
//     initialized at creation: an uninitialized capture is seeded
//     uninitialized, so the body analysis reports the read (RFC-001 §48–49,
//     RFC-003 §82);
//   - a non-nil capture is seeded with its narrowing (CONTRACTS §1.5, the
//     narrowing analogue of the initialization seeding);
//   - a write-only capture of an uninitialized binding is allowed and its
//     assignment initializes it inside the closure flow (RFC-003 §83);
//   - closure operations never update caller facts (RFC-003 §84, §170.31).
//
// It returns the bindings the body assigns without declaring - the mutated
// captures, for §85 call invalidation - and whether the body can fall off
// its end without a value-return (the D-6 measurement input, RFC-001
// §13.18; meaningful only for declared non-null results).
func (b *builder) analyzeClosure(cl *parser.Closure, scope *semantic.Scope, fallible bool) ([]semantic.BindingID, bool) {
	bodyScope := scope.Child()
	var entry []semantic.Operation
	paramIDs := map[semantic.BindingID]bool{}
	for _, p := range cl.Params {
		if serr := bodyScope.Declare(p.Name); serr != nil {
			b.report(serr.Category, cl.Span)
			continue
		}
		id := bodyScope.Resolve(p.Name)
		paramIDs[id] = true
		// §27 stable bindings: parameter types carry the declared class
		// (story 08) - a `T?` parameter gates ordinary member access.
		b.classes[id] = nullabilityOfParam(p)
		// Story 22: named parameter types root field-path classification.
		if p.TypeExpr != nil && p.TypeExpr.Kind == parser.NamedType {
			b.bindingTypes[id] = p.TypeExpr.Name
		} else if p.TypeExpr != nil && p.TypeExpr.Kind == parser.PointerType &&
			p.TypeExpr.Elem != nil && p.TypeExpr.Elem.Kind == parser.NamedType {
			// Story 40: pointer parameters name their type for conversion
			// checks and per-type method resolution (classes untouched).
			b.bindingTypes[id] = "*" + p.TypeExpr.Elem.Name
		}
		entry = append(entry, semantic.Declare(id), semantic.Assign(id))
	}
	seen := map[semantic.BindingID]bool{}
	for _, ident := range closureIdents(cl.Body) {
		id := bodyScope.Resolve(ident)
		if id == 0 || paramIDs[id] || seen[id] {
			continue
		}
		seen[id] = true
		entry = append(entry, semantic.Declare(id))
		if b.isInitialized(id) {
			entry = append(entry, semantic.Assign(id))
		}
		if b.isNonNil(id) {
			entry = append(entry, semantic.Assume(id))
		}
	}
	cb := &builder{
		blocks:       []semantic.Block{{ID: 1, Operations: entry}},
		facts:        []map[semantic.BindingID]bool{{}},
		nonNil:       []map[semantic.BindingID]bool{{}},
		pathNN:       []map[string]bool{{}},
		fallible:     fallible,
		successCount: successCount(cl),
		nextBlockID:  1,
		nilable:      b.nilable,
		classes:      b.classes,
		flowNullable: b.flowNullable,
		declared:     map[semantic.BindingID]bool{},
		assigned:     map[semantic.BindingID]bool{},
		errSpans:     map[semantic.BindingID]parser.Span{},
		reads:        map[semantic.BindingID]bool{},
		pure:         b.pure,
		// Story 22: the field model maps are shared read-only (the closure
		// body reads field classes; its own declarations register in fresh
		// maps). Story 35: funcResults/funcParams are shared too - try
		// operands and D-3 argument checks inside a function body resolve
		// the functions declared before the enclosing one; nested
		// declarations do not exist (the parser rejects them).
		structs:      b.structs,
		enums:        b.enums,
		bindingTypes: map[semantic.BindingID]string{},
		funcParams:   b.funcParams,
		funcResults:  b.funcResults,
		// Story 37: the method table is shared too - D-1/D-3 and purity
		// for method calls inside a function body resolve the methods
		// declared before the enclosing one (same rationale as
		// funcResults in story 35).
		methods:       b.methods,
		methodMutates: b.methodMutates,
		// Story 36: correlation does not cross the closure boundary (the
		// guard-kill-in-captures question is a follow-up) - the body
		// starts with no conditional bindings; terminated tracking is
		// per-body.
		correlated: map[semantic.BindingID]semantic.BindingID{},
		terminated: []bool{false},
	}
	cb.emit(cl.Body, bodyScope)
	// Closure bodies participate in the same lint and read accounting:
	// a read anywhere lifts the warning, a closure-local `error?`
	// declaration is linted with the rest (CONTRACTS §3).
	for id := range cb.reads {
		b.reads[id] = true
	}
	for id, span := range cb.errSpans {
		b.errSpans[id] = span
	}
	var mutators []semantic.BindingID
	for id := range cb.assigned {
		if !cb.declared[id] {
			mutators = append(mutators, id)
		}
	}
	b.diagnostics = append(b.diagnostics, cb.diagnostics...)
	b.diagnostics = append(b.diagnostics, cb.analyze().Diagnostics...)
	return mutators, semantic.FallOffEnd(cb.cfg())
}

// propagateMutators implements Решение 2 (2026-09-16): assigning a closure
// value held by another binding propagates its mutator set to the target
// (`var c2 = clear; c2()` invalidates the narrowing). Only the unambiguous
// single-target bare-identifier form propagates; call values, navigation
// chains and compound values are not aliasing sources.
func (b *builder) propagateMutators(statement *parser.Statement, scope *semantic.Scope, targets []semantic.BindingID) {
	if len(statement.Values) != 1 || len(targets) != 1 {
		return
	}
	value := statement.Values[0]
	if value.Closure != nil || value.Navigation != nil || len(value.Idents) != 1 || value.Text != value.Idents[0] {
		return
	}
	source := scope.Resolve(value.Idents[0])
	if source == 0 || source == targets[0] {
		return
	}
	if mutators := b.closureMutates[source]; len(mutators) > 0 {
		b.closureMutates[targets[0]] = unionBindings(b.closureMutates[targets[0]], mutators)
	}
}

func unionBindings(existing, added []semantic.BindingID) []semantic.BindingID {
	seen := make(map[semantic.BindingID]bool, len(existing)+len(added))
	out := make([]semantic.BindingID, 0, len(existing)+len(added))
	for _, id := range existing {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range added {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// emitCall lowers a call statement: arguments evaluate first (RFC-003 §61),
// the bare callee is a binding read, and the call of a declared function or
// closure applies its mutator set (RFC-003 §85). A navigation call requires
// the non-nil proof for its ordinary prefix and invalidates the receiver's
// narrowing after the call (story 07, решение 3b — the Rust &mut self
// analogue); its method name is never a binding read (F-G3 boundary). An
// annotated `//anuy:pure` callee applies no mutator set (trusted contract).
func (b *builder) emitCall(statement *parser.Statement, scope *semantic.Scope) {
	call := statement.Call
	if len(call.Segments) == 0 && parser.IntrinsicNames[call.Receiver] {
		// Story 41 (RFC-007 §6.6.5): the compiler intrinsic is not an
		// ordinary callee - name resolution never sees it.
		b.emitAssumeNonNull(statement, scope)
		return
	}
	if len(call.Segments) == 0 && b.unsafeFuncs[call.Receiver] && b.unsafeDepth == 0 {
		// Story 41 (RFC-007 §8.2.2 D-2): the caller must uphold the
		// function's safety contract - the call requires the context.
		// Error-handling rules still apply below (§6.8.4).
		b.report(semantic.UnsafeCallOutsideContext, statement.Span)
	}
	// Story 37 (RFC-005 §6.6.1, D-1): a call statement silently drops its
	// error result. `discard` is the explicit opt-out (§6.6.2); `try`
	// propagates instead of ignoring (story 34) - neither reports.
	if !statement.Discard && statement.Kind != parser.Try {
		if res, ok := b.calleeResults(call, scope); ok && res.fallible {
			b.report(semantic.IgnoredError, statement.Span)
		}
	}
	// A bare-identifier argument is a read (D-01): unresolved, it reports
	// exactly one UnknownRead per name. Compound and navigation arguments
	// keep the readIdents tolerance - unresolved idents there stay invisible
	// (recorded conformance sources pin them as accepted).
	reported := map[string]bool{}
	for _, value := range statement.Values {
		if value.Navigation != nil || value.Closure != nil || len(value.Idents) != 1 || value.Text != value.Idents[0] {
			continue
		}
		if serr := scope.Read(value.Idents[0]); serr != nil && !reported[value.Idents[0]] {
			reported[value.Idents[0]] = true
			b.report(serr.Category, statement.Span)
		}
	}
	b.readIdents(statement, scope)
	b.analyzeClosures(statement, scope, nil)
	b.checkArgumentTypes(statement, scope)
	id := scope.Resolve(call.Receiver)
	if id == 0 {
		if len(call.Segments) == 0 {
			b.report(semantic.UnknownRead, statement.Span)
		}
		return
	}
	b.add(semantic.Read(id))
	if len(call.Segments) > 0 {
		if call.Segments[0].Safe {
			// D-4 (RFC-002 §8.2.4, ADR-0005): a safe call on a receiver
			// known to be non-null is redundant.
			if b.knownNonNull(id) {
				b.report(semantic.RedundantSafeNavigation, call.Segments[0].Span)
			}
		} else if b.needsNonNilProof(id) {
			b.add(semantic.Deref(id))
		}
		// Story 22/25: a safe segment behind an ordinary field path is
		// redundant when the prefix path is known non-null by declared
		// classes (6.3.7).
		if safeAt := safeSegmentIndex(call); safeAt > 0 {
			if fields, ok := pathFieldsPrefix(call, safeAt); ok && b.pathKnownNonNull(id, fields) {
				b.report(semantic.RedundantSafeNavigation, call.Segments[safeAt].Span)
			}
		}
		// A resolved method (story 08): purity opts out of both effects
		// (Q2-A) - no receiver invalidation (3b), no capture mutators (3a);
		// otherwise the call invalidates the receiver and the method's
		// mutated captures.
		if key, info, ok := b.resolveMethod(call.Segments[len(call.Segments)-1].Name, id); ok {
			if !info.pure {
				mutators := append([]semantic.BindingID{id}, b.methodMutates[key]...)
				b.add(semantic.Call(mutators...))
			}
			return
		}
		// Story 40 (RFC-004 §8.1.5 D-5): an interface-typed receiver has
		// an exact method set - a member outside it is a definite error,
		// not tolerance (unknown concrete receivers stay tolerated).
		if b.interfaces[b.bindingTypes[id]] != nil {
			b.report(semantic.UndefinedInterfaceMember, call.Segments[len(call.Segments)-1].Span)
		}
		// Unresolved member call: the conservative receiver invalidation
		// stays (story 07, решение 3b - the Rust &mut self analogue).
		b.add(semantic.Call(id))
		return
	}
	if mutators := b.closureMutates[id]; len(mutators) > 0 && !b.pure[id] {
		b.add(semantic.Call(mutators...))
	}
}

// checkArgumentTypes reports D-3 (RFC-002 §8.2.3, story 18): a
// classified-null argument on a declared non-null parameter. Functions
// resolve through funcParams, methods through the flat table's parameter
// classes; unresolved callees, unpaired extra arguments and unknown
// classes stay unchecked (conservative). Widening is free (§6.2.1).
func (b *builder) checkArgumentTypes(statement *parser.Statement, scope *semantic.Scope) {
	call := statement.Call
	var params []semantic.Nullability
	if len(call.Segments) == 0 {
		params = b.funcParams[call.Receiver]
	} else {
		if _, info, ok := b.resolveMethod(call.Segments[len(call.Segments)-1].Name, scope.Resolve(call.Receiver)); ok {
			params = info.params
		}
	}
	if len(params) == 0 {
		return
	}
	for i := range statement.Values {
		if i >= len(params) || params[i] != semantic.NullabilityNonNull {
			continue
		}
		if b.classifyValue(&statement.Values[i], scope) == semantic.NullabilityNullable {
			b.report(semantic.NullableArgument, statement.Span)
		}
	}
}

// emitAssumeNonNull applies the assume_non_nil intrinsic (story 41,
// RFC-007 §6.6.5-§6.6.11): outside an unsafe context it is rejected
// (§8.2.1 D-1); inside, the operand stays an ordinary read and the
// non-null fact attaches to the operand binding. An already-proven
// operand warns redundant (§8.2.6 R) - proof beats assertion.
func (b *builder) emitAssumeNonNull(statement *parser.Statement, scope *semantic.Scope) {
	if len(statement.Values) != 1 {
		return
	}
	operand := statement.Values[0]
	if b.unsafeDepth == 0 {
		if len(operand.Idents) == 1 && operand.Text == operand.Idents[0] {
			if serr := scope.Read(operand.Idents[0]); serr != nil {
				b.report(serr.Category, statement.Span)
			}
		}
		b.report(semantic.UnsafeOperationOutside, statement.Span)
		return
	}
	if len(operand.Idents) != 1 || operand.Text != operand.Idents[0] {
		// Compound operands stay in the tolerance zone (CONTRACTS): the
		// value shape analysis has no projection for them.
		return
	}
	if serr := scope.Read(operand.Idents[0]); serr != nil {
		b.report(serr.Category, statement.Span)
		return
	}
	id := scope.Resolve(operand.Idents[0])
	if id == 0 {
		return
	}
	if b.knownNonNull(id) {
		b.report(semantic.RedundantUnsafeAssertion, statement.Span)
		return
	}
	b.assume(id)
}

// checkUnsafeCallValues reports D-2 (RFC-007 §8.2.2) for unsafe-func
// calls in initializer and assignment positions, which never reach
// emitCall (story 41). Statement calls are covered there.
func (b *builder) checkUnsafeCallValues(values []parser.Value, span parser.Span) {
	if b.unsafeDepth > 0 {
		return
	}
	for i := range values {
		if prefix := callCalleePrefix(values[i].Text); prefix != "" && b.unsafeFuncs[prefix] {
			b.report(semantic.UnsafeCallOutsideContext, span)
			return
		}
	}
}

// emitFunction lowers a story 07 function declaration: the declared name
// binds a closure value, so the body is analyzed at the creation point like
// a closure literal - self-recursion resolves through the scope, and the
// body's mutated captures register in the mutator registry (CONTRACTS
// story 07 §1). `//anuy:pure` marks the trusted no-writes contract. The
// declared result type (story 08 Q4-A) feeds the RHS call-shape
// classification. The method form dispatches to emitMethod.
// isFallible reports whether a declared result makes the function
// fallible (story 34, RFC-005 §6.2.3): the trailing result is `error?`.
func isFallible(statement *parser.Statement) bool {
	fallible, _ := strictResults(statement)
	return fallible
}

// strictResults resolves the strict fallible shape of a declaration
// (story 35, RFC-005 §6.2.3): the fallible flag plus the per-success-result
// nullability classes. A parenthesized result list is fallible when its
// trailing element is `error?` (the parse layer gates the spelling); the
// single-type spelling mirrors the story 34 rule. Success classes follow
// the G1 declared-class rule: named types classify by their `?`, composite
// spellings stay unknown.
func strictResults(statement *parser.Statement) (bool, []semantic.Nullability) {
	cl := statement.Closure
	if cl != nil && len(cl.ResultList) > 0 {
		last := cl.ResultList[len(cl.ResultList)-1]
		fallible := last.Kind == parser.NamedType && last.Name == "error" && last.Nullable
		var classes []semantic.Nullability
		for _, t := range cl.ResultList[:len(cl.ResultList)-1] {
			classes = append(classes, nullabilityOfTypeExpr(t))
		}
		return fallible, classes
	}
	fallible := statement.HasResult && statement.ResultNullable &&
		statement.Closure != nil && statement.Closure.ResultTypeExpr != nil &&
		statement.Closure.ResultTypeExpr.Kind == parser.NamedType &&
		statement.Closure.ResultTypeExpr.Name == "error"
	return fallible, nil
}

// nullabilityOfTypeExpr classifies a structural type expression by the G1
// declared-class rule (story 08): a named type is non-null or nullable by
// its `?`, every composite spelling stays unknown.
func nullabilityOfTypeExpr(t *parser.TypeExpr) semantic.Nullability {
	if t == nil || t.Kind != parser.NamedType {
		return semantic.NullabilityUnknown
	}
	if t.Nullable {
		return semantic.NullabilityNullable
	}
	return semantic.NullabilityNonNull
}

// successCount counts the strict fallible success results of a declaration
// body (story 35, RFC-005 §6.2.3): 0 for error-only and non-fallible
// shapes - the D-7 mixed-return gate applies only to strict functions.
func successCount(cl *parser.Closure) int {
	if cl == nil || len(cl.ResultList) < 2 {
		return 0
	}
	return len(cl.ResultList) - 1
}

func (b *builder) emitFunction(statement *parser.Statement, scope *semantic.Scope) {
	if statement.Method != "" {
		b.emitMethod(statement, scope)
		return
	}
	name := statement.Names[0]
	if statement.UnsafeFunc {
		// Story 41 (RFC-007 §6.7.1): calls require an unsafe context; the
		// body is NOT an implicit unsafe block (§6.7.3) - the closure
		// body below analyzes at depth 0.
		b.unsafeFuncs[name] = true
	}
	var paramClasses []semantic.Nullability
	for _, p := range statement.Closure.Params {
		paramClasses = append(paramClasses, nullabilityOfParam(p))
	}
	b.funcParams[name] = paramClasses
	// Story 39 (RFC-004 §6.1.5/§6.1.6): functions and methods live in
	// separate namespaces - a bare call resolves the function, a receiver
	// call the per-type method set; the flat Q1-A collision is gone.
	if serr := scope.Declare(name); serr != nil {
		b.report(serr.Category, statement.Span)
		return
	}
	id := scope.Resolve(name)
	b.declare(id)
	b.initialize(id)
	fallible, successClasses := strictResults(statement)
	b.funcResults[name] = declResult{hasResult: statement.HasResult, resultNullable: statement.ResultNullable, fallible: fallible, successClasses: successClasses}
	mutators, fallOff := b.analyzeClosure(statement.Closure, scope, fallible)
	b.closureMutates[id] = unionBindings(b.closureMutates[id], mutators)
	if statement.Pure {
		b.pure[id] = true
	}
	// D-6 (RFC-001 §13.18, ADR-0004): a declared non-null result must be
	// initialized on every exit path; a nullable result falls off to
	// semantic nil, a valid value.
	if statement.HasResult && !statement.ResultNullable && fallOff {
		b.report(semantic.MissingReturn, statement.Span)
	}
	// Story 35 (RFC-005 §6.4.7): a strict fallible function may not fall
	// off its end - neither success (`v, nil`) nor failure is implied, and
	// the success results have no value to fabricate. Error-only fall-off
	// stays an implicit `return nil`.
	if fallible && len(successClasses) > 0 && fallOff {
		b.report(semantic.MissingReturn, statement.Span)
	}
}

// emitMethod lowers a story 08 method declaration (RFC-002 §40 form):
// methods bind no scope name - the per-type method set resolves calls
// (story 39, RFC-004 §6.1.5); a duplicate within one type rejects
// (§6.1.6 - the pointer spelling does not create a second namespace).
// The body analyzes through the closure path (receiver access is not
// modeled in v1); its mutated captures register as the method's 3a
// mutator set. `//anuy:pure` opts the method out of both call effects
// (Q2-A, F-C2).
func (b *builder) emitMethod(statement *parser.Statement, scope *semantic.Scope) {
	name := statement.Names[0]
	if statement.UnsafeFunc {
		// Story 41 (RFC-007 §6.7.1): calls require an unsafe context; the
		// body is NOT an implicit unsafe block (§6.7.3) - the closure
		// body below analyzes at depth 0.
		b.unsafeFuncs[name] = true
	}
	var paramClasses []semantic.Nullability
	for _, p := range statement.Closure.Params {
		paramClasses = append(paramClasses, nullabilityOfParam(p))
	}
	key := methodKey(statement.Method, name)
	if _, exists := b.methods[key]; exists {
		b.report(semantic.SameScopeRedeclaration, statement.Span)
		return
	}
	fallible, successClasses := strictResults(statement)
	b.methods[key] = methodInfo{
		hasResult:      statement.HasResult,
		resultNullable: statement.ResultNullable,
		pure:           statement.Pure,
		params:         paramClasses,
		fallible:       fallible,
		successClasses: successClasses,
		receiver:       statement.Method,
		pointer:        statement.MethodPointer,
	}
	// Registered before the body: a self-recursive call resolves like a
	// function's self-recursion (story 07 precedent; the mutator set the
	// self-call sees stays the one known at that point).
	mutators, fallOff := b.analyzeClosure(statement.Closure, scope, fallible)
	b.methodMutates[key] = unionBindings(nil, mutators)
	// D-6 (RFC-001 §13.18, ADR-0004): methods enforce missing-return like
	// functions; story 35 adds the §6.4.7 strict fallible fall-off.
	if statement.HasResult && !statement.ResultNullable && fallOff {
		b.report(semantic.MissingReturn, statement.Span)
	}
	if fallible && len(successClasses) > 0 && fallOff {
		b.report(semantic.MissingReturn, statement.Span)
	}
}

// emitTryDecl analyzes the value-try declaration `var v = try F(x)` and
// `var a, b = try Op()` (story 35, RFC-005 §6.5.1–6.5.2). The enclosing
// function must be fallible (ANUY6001, §6.5.4); the callee must carry the
// trailing `error?` (D-2, ANUY6003); the binding count must equal the
// callee's success results; every binding is definitely initialized after
// the declaration (§6.9.2 - try eliminates the conditional state), with
// the declared class of its success result.
func (b *builder) emitTryDecl(statement *parser.Statement, scope *semantic.Scope) {
	if !b.fallible {
		b.report(semantic.PropagationOutsideFallible, statement.Span)
	}
	b.readIdents(statement, scope)
	b.analyzeClosures(statement, scope, nil)
	res, ok := b.calleeResults(statement.TryCall, scope)
	if ok && !res.fallible {
		b.report(semantic.InvalidTry, statement.Span)
	}
	if ok && res.fallible && len(statement.Names) != len(res.successClasses) {
		// One binding per success result (RFC-003 multi-value declaration;
		// the ANUY1006 registry category, parser-block code).
		b.report(semantic.DiagnosticCategory("ArityMismatch"), statement.Span)
	}
	if ok && res.fallible {
		targets := make([]semantic.BindingID, 0, len(statement.Names))
		for i, name := range statement.Names {
			if name == "_" {
				// GB-3 variant A: the blank target receives its success
				// value without creating a binding.
				continue
			}
			if serr := scope.Declare(name); serr != nil {
				b.report(serr.Category, statement.Span)
				continue
			}
			id := scope.Resolve(name)
			targets = append(targets, id)
			b.declare(id)
			if i < len(res.successClasses) {
				b.classes[id] = res.successClasses[i]
				if res.successClasses[i] == semantic.NullabilityNullable {
					b.nilable[id] = true
				}
			}
		}
		// §6.9.2: the success path proves every success result exists -
		// definite initialization, no conditional state (the §6.3
		// correlation slice introduces that).
		for _, id := range targets {
			b.initialize(id)
		}
	}
}

// checkFailureOperand reports D-5 (RFC-005 §8.1.5, story 35): the
// `return error` operand must be a proven non-null error. A literal `nil`
// operand is never a failure; a nullable binding needs a live narrowing
// fact (`if err != nil`); unknown classes stay trusted (the §6.4.2
// `errors.New(...)` shape).
func (b *builder) checkFailureOperand(statement *parser.Statement, scope *semantic.Scope) {
	if len(statement.Values) == 1 && statement.Values[0].Text == "nil" {
		b.report(semantic.InvalidFailureReturn, statement.Span)
		return
	}
	for _, value := range statement.Values {
		for _, ident := range value.Idents {
			if ident == "nil" {
				continue
			}
			id := scope.Resolve(ident)
			if id != 0 && b.classes[id] == semantic.NullabilityNullable && !b.nonNil[b.cur][id] {
				b.report(semantic.InvalidFailureReturn, statement.Span)
				return
			}
		}
	}
}

// checkStrictReturn reports D-7 (RFC-005 §6.4.5/§8.1.7, story 35): in a
// strict fallible function the ordinary return must spell the success form
// `v1, …, nil` - the trailing slot is the literal `nil`, never an error
// value. The failure form is checked by D-5; the bare return is the
// fall-off case (§6.4.7, measured at the function end).
func (b *builder) checkStrictReturn(statement *parser.Statement, scope *semantic.Scope) {
	if len(statement.Values) == 0 {
		return
	}
	if statement.Values[len(statement.Values)-1].Text != "nil" {
		b.report(semantic.MixedReturn, statement.Span)
	}
}

// callCalleePrefix extracts the callee prefix (`Load`, `obj.method`) of a
// raw call value text; empty when the value is not a call shape.
func callCalleePrefix(text string) string {
	open := strings.Index(text, "(")
	if open <= 0 {
		return ""
	}
	return strings.TrimSpace(text[:open])
}

// emitVarDestructuring analyzes the fallible-call destructuring
// `var data, err = Load()` (story 36, RFC-005 §6.3, §6.9.1): the trailing
// binding receives the error result (initialized, `error?` class, R1
// tracked), every preceding binding is conditionally initialized - guard
// `(err == nil)` (§6.3.1). Reports handled=false for any other shape:
// unknown or non-fallible callees keep the generic unconditional path
// (F-G3), a result-count mismatch reports ANUY1006 and recovers the same
// way.
func (b *builder) emitVarDestructuring(statement *parser.Statement, scope *semantic.Scope) bool {
	res, ok := b.calleeResultsForPrefix(callCalleePrefix(statement.Values[0].Text))
	if !ok || !res.fallible {
		return false
	}
	b.readIdents(statement, scope)
	b.analyzeClosures(statement, scope, nil)
	if len(statement.Names) != len(res.successClasses)+1 {
		b.report(semantic.DiagnosticCategory("ArityMismatch"), statement.Span)
		return false
	}
	errID := semantic.BindingID(0)
	errName := statement.Names[len(statement.Names)-1]
	if errName == "_" {
		// Story 37 (RFC-005 §6.6.5, D-1): blank does not bypass
		// must-consume - the error result is silently dropped. The value
		// bindings turn dead-guard correlated: data exists on no path,
		// reads report D-4, no later proof can materialize them.
		b.report(semantic.IgnoredError, statement.Span)
	} else {
		if serr := scope.Declare(errName); serr != nil {
			b.report(serr.Category, statement.Span)
			return false
		}
		errID = scope.Resolve(errName)
		b.declare(errID)
		b.classes[errID] = semantic.NullabilityNullable
		b.nilable[errID] = true
		b.errSpans[errID] = statement.Span
		b.initialize(errID)
	}
	success := statement.Names[:len(statement.Names)-1]
	targets := make([]semantic.BindingID, 0, len(success))
	for i, name := range success {
		if name == "_" {
			// GB-3 variant A: the blank success target receives nothing
			// and creates no binding.
			continue
		}
		if serr := scope.Declare(name); serr != nil {
			b.report(serr.Category, statement.Span)
			continue
		}
		id := scope.Resolve(name)
		targets = append(targets, id)
		b.declare(id)
		// Story 36: the op stream stays analyzer-optimistic - the
		// declaration carries Assign, so the kernel never fires
		// ANUY3001 for a correlated binding; the builder facts keep the
		// real conditional state and D-4 is the sound diagnostic.
		b.add(semantic.Assign(id))
		b.assigned[id] = true
		if i < len(res.successClasses) {
			b.classes[id] = res.successClasses[i]
			if res.successClasses[i] == semantic.NullabilityNullable {
				b.nilable[id] = true
			}
		}
	}
	// Entries register after the guard exists and is initialized:
	// initialize() kills dependents (§6.3.6) and must not fire here.
	// A blank error target leaves errID = 0 (story 37, §6.6.5) - the
	// dead guard keeps reads reporting D-4 and never materializes.
	for _, id := range targets {
		b.correlated[id] = errID
	}
	return true
}

// calleeResults resolves the strict fallible shape of a try operand (story
// 35): plain calls through funcResults, method calls through the per-type
// method sets (story 39). ok=false marks the tolerance zone - an
// unresolved callee stays unchecked (F-G3).
func (b *builder) calleeResults(call *parser.NavigationExpr, scope *semantic.Scope) (declResult, bool) {
	if call == nil {
		return declResult{}, false
	}
	if len(call.Segments) == 0 {
		if res, ok := b.funcResults[call.Receiver]; ok {
			return res, true
		}
		return declResult{}, false
	}
	if _, info, ok := b.resolveMethod(call.Segments[len(call.Segments)-1].Name, scope.Resolve(call.Receiver)); ok {
		return declResult{hasResult: info.hasResult, resultNullable: info.resultNullable, fallible: info.fallible, successClasses: info.successClasses}, true
	}
	return declResult{}, false
}

// calleeResultsForPrefix is the raw-text mirror of calleeResults (story
// 36): a destructuring RHS keeps its call as raw text (`Load(p)`), so the
// callee prefix resolves through the same tables. ok=false marks the
// tolerance zone (F-G3).
func (b *builder) calleeResultsForPrefix(prefix string) (declResult, bool) {
	if prefix == "" {
		return declResult{}, false
	}
	if i := strings.LastIndex(prefix, "."); i >= 0 {
		if info, ok := b.methods[methodKey(prefix[:i], prefix[i+1:])]; ok {
			return declResult{hasResult: info.hasResult, resultNullable: info.resultNullable, fallible: info.fallible, successClasses: info.successClasses}, true
		}
		if _, info, ok := b.resolveMethod(prefix[i+1:], 0); ok {
			return declResult{hasResult: info.hasResult, resultNullable: info.resultNullable, fallible: info.fallible, successClasses: info.successClasses}, true
		}
		return declResult{}, false
	}
	if res, ok := b.funcResults[prefix]; ok {
		return res, true
	}
	return declResult{}, false
}

// emitReturn lowers the `return` statement: the flow of the enclosing
// function terminates here. A `return expr` value (story 08) evaluates
// before the exit - its reads join the flow (they lift the R1 lint and
// carry deref gating), closure values analyze at the return point, and
// the Return operation marks the flow terminator for the D-6 measurement
// (RFC-001 §13.18). A bare `return` provides no result, so it does not
// terminate: a result function ending in one still falls off (D-6); in
// void bodies the measurement does not apply. Following statements start
// a fresh block with no incoming edges - the analyzer skips unreachable
// blocks, so their reads report nothing.
func (b *builder) emitReturn(statement *parser.Statement, scope *semantic.Scope) {
	// Story 34 (RFC-005 §6.4.2): the failure return requires a fallible
	// enclosing function.
	if statement.ErrorReturn && !b.fallible {
		b.report(semantic.PropagationOutsideFallible, statement.Span)
	}
	// Story 35 (RFC-005 §6.4.2/§8.1.5, D-5): the failure-return operand
	// must be a proven non-null error.
	if statement.ErrorReturn && b.fallible {
		b.checkFailureOperand(statement, scope)
	}
	// Story 35 (RFC-005 §6.4.5/§8.1.7, D-7): a strict fallible success
	// return must terminate in the literal `nil` error slot.
	if b.successCount > 0 && !statement.ErrorReturn {
		b.checkStrictReturn(statement, scope)
	}
	if len(statement.Values) > 0 {
		b.readIdents(statement, scope)
		b.analyzeClosures(statement, scope, nil)
		b.add(semantic.Return())
	}
	cur := b.cur
	b.appendBlock()
	b.facts[b.cur] = copyFacts(b.facts[cur])
	b.nonNil[b.cur] = copyFacts(b.nonNil[cur])
	// Story 36: flow does not continue past a return - the fresh block is
	// the terminated branch exit emitIf excludes from the join (§6.3.2).
	b.terminated[b.cur] = true
}

// condNarrowTarget resolves the binding narrowed by a bare `x != nil`
// condition - the only nil-comparison form in the experimental slice;
// conditions otherwise stay raw text until the general expression grammar
// exists. Zero means the condition carries no narrowing.
func (b *builder) condNarrowTarget(statement *parser.Statement, scope *semantic.Scope) semantic.BindingID {
	parts := strings.SplitN(statement.Cond, "!=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return 0
	}
	name := strings.TrimSpace(parts[0])
	for _, ident := range statement.CondIdents {
		if ident == name {
			return scope.Resolve(ident)
		}
	}
	return 0
}

// closureIdents collects identifiers referenced anywhere in the statements,
// including nested closures, in source order. Assignment and declaration
// targets are not reads and are not collected.
func closureIdents(statements []parser.Statement) []string {
	var out []string
	for i := range statements {
		s := &statements[i]
		out = append(out, s.CondIdents...)
		for _, value := range s.Values {
			if value.Closure == nil {
				out = append(out, value.Idents...)
				continue
			}
			out = append(out, closureIdents(value.Closure.Body)...)
		}
		if s.Kind == parser.Read {
			out = append(out, s.Names...)
		}
		// A call statement's bare callee and navigation receiver are binding
		// reads (CONTRACTS §2): they participate in capture seeding. Story
		// 34: the same for the try statement's propagated call. Story 35:
		// the value-try declaration's operand call.
		if (s.Kind == parser.Call || s.Kind == parser.Try) && s.Call != nil {
			out = append(out, s.Call.Receiver)
		}
		if s.TryCall != nil {
			out = append(out, s.TryCall.Receiver)
		}
		// Story 31/33: a switch reads its scrutinee and its arm bodies
		// participate in capture seeding.
		if s.Switch != nil {
			out = append(out, s.Switch.Scrutinee)
			for _, arm := range s.Switch.Arms {
				out = append(out, closureIdents(arm.Body)...)
			}
		}
		out = append(out, closureIdents(s.Body)...)
		out = append(out, closureIdents(s.Else)...)
	}
	return out
}

func copyFacts(facts map[semantic.BindingID]bool) map[semantic.BindingID]bool {
	out := make(map[semantic.BindingID]bool, len(facts))
	for id, initialized := range facts {
		out[id] = initialized
	}
	return out
}

// copyPathFacts mirrors copyFacts for the path narrowing facts (story 27).
func copyPathFacts(facts map[string]bool) map[string]bool {
	out := make(map[string]bool, len(facts))
	for key, value := range facts {
		out[key] = value
	}
	return out
}

// intersectFacts keeps a binding initialized only if both incoming paths
// initialize it, matching the kernel join semantics.
func intersectFacts(a, b map[semantic.BindingID]bool) map[semantic.BindingID]bool {
	out := make(map[semantic.BindingID]bool)
	for id, initialized := range a {
		if initialized && b[id] {
			out[id] = true
		}
	}
	return out
}

// intersectPathFacts keeps a path fact only if both incoming paths carry
// it - the join semantics of the story 27 narrowing (a fact proven on one
// branch alone does not survive).
func intersectPathFacts(a, b map[string]bool) map[string]bool {
	out := make(map[string]bool)
	for key, value := range a {
		if value && b[key] {
			out[key] = true
		}
	}
	return out
}

// pathFactKey builds the normalized narrowing-fact key of a path prefix
// (story 27, ADR-0008).
func pathFactKey(receiver string, fields []string) string {
	return receiver + "." + strings.Join(fields, ".")
}

// condNarrowPath resolves the field path narrowed by a bare
// `u.link != nil` condition (story 27, ADR-0008) - the path mirror of
// condNarrowTarget. Empty when the condition carries no path narrowing.
func (b *builder) condNarrowPath(statement *parser.Statement, scope *semantic.Scope) string {
	parts := strings.SplitN(statement.Cond, "!=", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[1]) != "nil" {
		return ""
	}
	lhs := strings.TrimSpace(parts[0])
	if !strings.Contains(lhs, ".") {
		return ""
	}
	segments := strings.Split(lhs, ".")
	for _, segment := range segments {
		if segment == "" || strings.ContainsAny(segment, " \t") {
			return ""
		}
	}
	for _, ident := range statement.CondIdents {
		if ident == segments[0] {
			if scope.Resolve(ident) != 0 {
				return strings.Join(segments, ".")
			}
			return ""
		}
	}
	return ""
}

// killPathFacts invalidates the narrowing facts of an assignment to
// target (story 27, ADR-0008): facts equal to the target or extending it
// die; the target's own prefixes survive - writing `u.link.badge` does
// not change `u.link`.
func (b *builder) killPathFacts(target string) {
	for key := range b.pathNN[b.cur] {
		if key == target || strings.HasPrefix(key, target+".") {
			delete(b.pathNN[b.cur], key)
		}
	}
}

// killAllPathFacts drops every path narrowing fact - the conservative
// effect of any call statement and of loop entry (ADR-0008).
func (b *builder) killAllPathFacts() {
	b.pathNN[b.cur] = map[string]bool{}
}

// callPreservesPathFacts reports whether the resolved callee is trusted
// pure (`//anuy:pure`, story 07): a pure call cannot invalidate the
// receiver, so the story 27 path facts survive. Unresolved and
// unannotated callees stay conservative (story 28, ADR-0008 follow-up);
// the callee resolution mirrors emitCall - flat method table by the last
// segment, bare calls through the scope binding.
func (b *builder) callPreservesPathFacts(statement *parser.Statement, scope *semantic.Scope) bool {
	call := statement.Call
	if call == nil {
		return false
	}
	if len(call.Segments) == 0 {
		if id := scope.Resolve(call.Receiver); id != 0 {
			return b.pure[id]
		}
		return false
	}
	last := call.Segments[len(call.Segments)-1]
	if _, info, ok := b.resolveMethod(last.Name, scope.Resolve(call.Receiver)); ok {
		return info.pure
	}
	return false
}

func (b *builder) report(category semantic.DiagnosticCategory, span parser.Span) {
	desc, ok := semantic.DescriptorFor(category)
	if !ok {
		panic("integration: unregistered diagnostic category " + category)
	}
	b.diagnostics = append(b.diagnostics, semantic.NewDiagnostic(desc, 0, semantic.SourceSpan{Start: span.Start, End: span.End}))
}

// nullabilityOfParam classifies a declared parameter type (story 08): a
// named type is non-null or nullable by its `?`; composite spellings stay
// unknown - their nullability binding is outside the slice.
func nullabilityOfParam(p parser.Param) semantic.Nullability {
	if p.TypeExpr != nil {
		if p.TypeExpr.Kind == parser.NamedType {
			if p.TypeExpr.Nullable {
				return semantic.NullabilityNullable
			}
			return semantic.NullabilityNonNull
		}
		return semantic.NullabilityUnknown
	}
	return nullabilityOfTypeName(p.Type)
}

// nullabilityOfTypeName classifies a raw type-name spelling: a simple name
// is non-null, the same with a trailing `?` is nullable, everything else
// (slice/map/pointer composites) stays unknown.
func nullabilityOfTypeName(t string) semantic.Nullability {
	base := strings.TrimSuffix(t, "?")
	if base == t {
		if isSimpleTypeName(t) {
			return semantic.NullabilityNonNull
		}
		return semantic.NullabilityUnknown
	}
	if isSimpleTypeName(base) {
		return semantic.NullabilityNullable
	}
	return semantic.NullabilityUnknown
}

func isSimpleTypeName(t string) bool {
	if t == "" {
		return false
	}
	for i, r := range t {
		if !unicode.IsLetter(r) && r != '_' && (i == 0 || !unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
