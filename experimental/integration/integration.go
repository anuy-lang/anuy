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
// call out of both effects (3a+3b).
type methodInfo struct {
	hasResult      bool
	resultNullable bool
	pure           bool
	params         []semantic.Nullability
}

// declResult mirrors a declared function result type (story 08 Q4-A).
type declResult struct {
	hasResult      bool
	resultNullable bool
}

type builder struct {
	blocks      []semantic.Block
	facts       []map[semantic.BindingID]bool // initialization facts at the end of each block
	nonNil      []map[semantic.BindingID]bool // non-nil narrowing facts, parallel to facts (story 05)
	pathNN      []map[string]bool             // non-nil narrowing facts of field paths, parallel to facts (story 27, ADR-0008)
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
}

func newBuilder() *builder {
	return &builder{
		blocks:         []semantic.Block{{ID: 1}},
		facts:          []map[semantic.BindingID]bool{{}},
		nonNil:         []map[semantic.BindingID]bool{{}},
		pathNN:         []map[string]bool{{}},
		nextBlockID:    1,
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
	}
}

func (b *builder) appendBlock() {
	b.nextBlockID++
	b.blocks = append(b.blocks, semantic.Block{ID: b.nextBlockID})
	b.facts = append(b.facts, map[semantic.BindingID]bool{})
	b.nonNil = append(b.nonNil, map[semantic.BindingID]bool{})
	b.pathNN = append(b.pathNN, map[string]bool{})
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
		return b.classifyNavigationValue(value.Navigation)
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
	if strings.HasSuffix(value.Text, "()") {
		// A raw call shape (`name()`, `a.b()`): a declared result type
		// classifies the value (story 08 Q4-A); unresolved and void calls
		// stay unknown (CONTRACTS §2.1).
		return b.classifyCallShape(strings.TrimSuffix(value.Text, "()"))
	}
	return semantic.NullabilityUnknown
}

// classifyNavigationValue classifies a selector-chain value: a plain
// member read has no model (no fields, §28) and stays unknown; a resolved
// method call carries its declared result (story 08 Q4-A), lifted to
// nullable through safe navigation (RFC-002 §38).
func (b *builder) classifyNavigationValue(nav *parser.NavigationExpr) semantic.Nullability {
	last := nav.Segments[len(nav.Segments)-1]
	if !last.Call {
		return semantic.NullabilityUnknown
	}
	info, ok := b.methods[last.Name]
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
		if info, ok := b.methods[prefix[i+1:]]; ok {
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
	case parser.Switch:
		b.emitSwitch(statement, scope)
	case parser.Var:
		b.readIdents(statement, scope)
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
	}
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
		mutators, _ := b.analyzeClosure(value.Closure, scope)
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
	b.appendBlock()
	thenID := b.blocks[b.cur].ID
	b.facts[b.cur] = copyFacts(before)
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

	hasElse := statement.Else != nil
	var elseID, elseExit semantic.BlockID
	var elseFacts, elseNN map[semantic.BindingID]bool
	var elsePath map[string]bool
	if hasElse {
		b.appendBlock()
		elseID = b.blocks[b.cur].ID
		b.facts[b.cur] = copyFacts(before)
		b.nonNil[b.cur] = copyFacts(beforeNN)
		b.pathNN[b.cur] = copyPathFacts(beforePath)
		b.emit(statement.Else, scope.Child())
		elseFacts = copyFacts(b.facts[b.cur])
		elseNN = copyFacts(b.nonNil[b.cur])
		elsePath = copyPathFacts(b.pathNN[b.cur])
		elseExit = b.blocks[b.cur].ID
	}

	joinFacts := intersectFacts(thenFacts, before)
	joinNN := intersectFacts(thenNN, beforeNN)
	joinPath := intersectPathFacts(thenPath, beforePath)
	if hasElse {
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
	b.connect(thenExit, joinID)
	if hasElse {
		b.connect(start, elseID)
		b.connect(elseExit, joinID)
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
	for _, value := range statement.Values {
		if value.Closure != nil {
			continue
		}
		for _, ident := range value.Idents {
			if id := scope.Resolve(ident); id != 0 {
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
func (b *builder) analyzeClosure(cl *parser.Closure, scope *semantic.Scope) ([]semantic.BindingID, bool) {
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
		// maps).
		structs:      b.structs,
		enums:        b.enums,
		bindingTypes: map[semantic.BindingID]string{},
		funcParams:   map[string][]semantic.Nullability{},
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
		if info, ok := b.methods[call.Segments[len(call.Segments)-1].Name]; ok {
			if !info.pure {
				mutators := append([]semantic.BindingID{id}, b.methodMutates[call.Segments[len(call.Segments)-1].Name]...)
				b.add(semantic.Call(mutators...))
			}
			return
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
		params = b.methods[call.Segments[len(call.Segments)-1].Name].params
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

// emitFunction lowers a story 07 function declaration: the declared name
// binds a closure value, so the body is analyzed at the creation point like
// a closure literal - self-recursion resolves through the scope, and the
// body's mutated captures register in the mutator registry (CONTRACTS
// story 07 §1). `//anuy:pure` marks the trusted no-writes contract. The
// declared result type (story 08 Q4-A) feeds the RHS call-shape
// classification. The method form dispatches to emitMethod.
func (b *builder) emitFunction(statement *parser.Statement, scope *semantic.Scope) {
	if statement.Method != "" {
		b.emitMethod(statement, scope)
		return
	}
	name := statement.Names[0]
	var paramClasses []semantic.Nullability
	for _, p := range statement.Closure.Params {
		paramClasses = append(paramClasses, nullabilityOfParam(p))
	}
	b.funcParams[name] = paramClasses
	// One flat namespace (story 08 Q1-A): a function name colliding with a
	// declared method rejects.
	if _, exists := b.methods[name]; exists {
		b.report(semantic.SameScopeRedeclaration, statement.Span)
		return
	}
	if serr := scope.Declare(name); serr != nil {
		b.report(serr.Category, statement.Span)
		return
	}
	id := scope.Resolve(name)
	b.declare(id)
	b.initialize(id)
	mutators, fallOff := b.analyzeClosure(statement.Closure, scope)
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
	b.funcResults[name] = declResult{hasResult: statement.HasResult, resultNullable: statement.ResultNullable}
}

// emitMethod lowers a story 08 method declaration (RFC-002 §40 form):
// methods bind no scope name - the flat table resolves calls by unique
// name; duplicates and collisions with scope names reject (Q1-A). The body
// analyzes through the closure path (receiver access is not modeled in
// v1); its mutated captures register as the method's 3a mutator set.
// `//anuy:pure` opts the method out of both call effects (Q2-A, F-C2).
func (b *builder) emitMethod(statement *parser.Statement, scope *semantic.Scope) {
	name := statement.Names[0]
	var paramClasses []semantic.Nullability
	for _, p := range statement.Closure.Params {
		paramClasses = append(paramClasses, nullabilityOfParam(p))
	}
	if _, exists := b.methods[name]; exists {
		b.report(semantic.SameScopeRedeclaration, statement.Span)
		return
	}
	if scope.Resolve(name) != 0 {
		b.report(semantic.SameScopeRedeclaration, statement.Span)
		return
	}
	b.methods[name] = methodInfo{
		hasResult:      statement.HasResult,
		resultNullable: statement.ResultNullable,
		pure:           statement.Pure,
		params:         paramClasses,
	}
	// Registered before the body: a self-recursive call resolves like a
	// function's self-recursion (story 07 precedent; the mutator set the
	// self-call sees stays the one known at that point).
	mutators, fallOff := b.analyzeClosure(statement.Closure, scope)
	b.methodMutates[name] = unionBindings(nil, mutators)
	// D-6 (RFC-001 §13.18, ADR-0004): methods enforce missing-return like
	// functions.
	if statement.HasResult && !statement.ResultNullable && fallOff {
		b.report(semantic.MissingReturn, statement.Span)
	}
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
	if len(statement.Values) > 0 {
		b.readIdents(statement, scope)
		b.analyzeClosures(statement, scope, nil)
		b.add(semantic.Return())
	}
	cur := b.cur
	b.appendBlock()
	b.facts[b.cur] = copyFacts(b.facts[cur])
	b.nonNil[b.cur] = copyFacts(b.nonNil[cur])
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
		// reads (CONTRACTS §2): they participate in capture seeding.
		if s.Kind == parser.Call && s.Call != nil {
			out = append(out, s.Call.Receiver)
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
	if info, ok := b.methods[last.Name]; ok {
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
