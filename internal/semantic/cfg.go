// Package semantic implements the dataflow kernel of the Anuy compiler:
// the CFG and its operations, definite-initialization and non-nil
// narrowing facts, scope resolution, and the diagnostic registry
// (RFC-001–003). The semantics are settled by the accepted RFCs and
// covered by conformance tests anchored to their sections; the exported
// API is internal and may still change without notice.
package semantic

// BindingID identifies a lexical binding across the kernel: scopes mint
// them, facts and operations refer to them.
type BindingID uint32

// BlockID identifies a CFG block; the entry block is always ID 1.
type BlockID uint32

// BindingState is the definite-initialization state of a binding.
type BindingState uint8

const (
	Uninitialized BindingState = iota
	Initialized
)

// Block is a straight-line sequence of operations; control flow between
// blocks is expressed solely by CFG edges.
type Block struct {
	ID         BlockID
	Operations []Operation
}

// OperationKind enumerates the kernel operations a block may carry.
type OperationKind uint8

const (
	DeclareOperation OperationKind = iota
	AssignOperation
	ReadOperation
	AssumeOperation
	DerefOperation
	CallOperation
	ReturnOperation
)

// Operation is one kernel step inside a block; most operations carry the
// binding they act on.
type Operation struct {
	Kind    OperationKind
	Binding BindingID
	// Mutates lists the bindings whose non-nil narrowing the call
	// invalidates; used only by CallOperation.
	Mutates []BindingID
}

// Declare, Assign and Read are the definite-initialization trio: a
// declaration starts a binding uninitialized, an assignment initializes
// it, a read requires it.
func Declare(binding BindingID) Operation { return Operation{Kind: DeclareOperation, Binding: binding} }
func Assign(binding BindingID) Operation  { return Operation{Kind: AssignOperation, Binding: binding} }
func Read(binding BindingID) Operation    { return Operation{Kind: ReadOperation, Binding: binding} }

// Assume establishes the non-nil narrowing fact for a binding: the lowering
// of a proven `x != nil` predicate (RFC-002 §22, story 05 CONTRACTS §1).
func Assume(binding BindingID) Operation { return Operation{Kind: AssumeOperation, Binding: binding} }

// Deref reads an ordinary member of a binding: it requires the non-nil
// narrowing fact and reports UnsafeMemberAccess when the proof is absent.
func Deref(binding BindingID) Operation { return Operation{Kind: DerefOperation, Binding: binding} }

// Call invokes a closure binding: it invalidates the non-nil narrowing of
// every binding the closure mutates (RFC-003 §85) and never establishes
// caller-local initialization (RFC-003 §84).
func Call(mutates ...BindingID) Operation {
	return Operation{Kind: CallOperation, Mutates: mutates}
}

// Return terminates the enclosing function's flow with its declared
// result (RFC-001 §13.18 rule 18, §8.2.6 D-6): it is the last operation
// of its block, and no flow continues past it.
func Return() Operation { return Operation{Kind: ReturnOperation} }

type DiagnosticCategory string

const (
	ReadBeforeInitialization   DiagnosticCategory = "ReadBeforeInitialization"
	PackageInitializerRequired DiagnosticCategory = "PackageInitializerRequired"
	// UnsafeMemberAccess is the experimental category for an ordinary member
	// access on a binding whose non-nil narrowing is not proven (story 05);
	// final naming and text belong to RFC-011.
	UnsafeMemberAccess DiagnosticCategory = "UnsafeMemberAccess"
	// MissingReturn is the D-6 category (RFC-001 §8.2.6, ADR-0004): a
	// declared non-null result is not initialized on all exit paths.
	MissingReturn DiagnosticCategory = "MissingReturn"
	// RedundantSafeNavigation is the D-4 category (RFC-002 §8.2.4/§12.10,
	// ADR-0005): a safe segment on a receiver known to be non-null at the
	// read point (a live narrowing fact or a declared non-null class).
	RedundantSafeNavigation DiagnosticCategory = "RedundantSafeNavigation"
)

// SourceSpan locates a diagnostic in the source text.
type SourceSpan struct {
	File       string
	Start, End int
}

// Diagnostic is one registry-stamped finding with its code, severity and
// span.
type Diagnostic struct {
	Category DiagnosticCategory
	Code     Code
	Severity Severity
	Binding  BindingID
	Span     SourceSpan
	// Related carries co-firing diagnostics suppressed as a cascade
	// (CONTRACTS §2, decision 4; RFC-011 §14 RelatedInformation).
	Related []Diagnostic
}

// NewDiagnostic stamps a diagnostic from a registry descriptor: the code
// and severity come from the registry, never from the call site
// (CONTRACTS §1.2).
func NewDiagnostic(desc Descriptor, binding BindingID, span SourceSpan) Diagnostic {
	return Diagnostic{
		Category: desc.Category(),
		Code:     desc.Code(),
		Severity: desc.Severity(),
		Binding:  binding,
		Span:     span,
	}
}

func ValidatePackageBinding(hasInitializer bool) *Diagnostic {
	if hasInitializer {
		return nil
	}
	d := NewDiagnostic(PackageInitializerRequiredDescriptor, 0, SourceSpan{})
	return &d
}

// AnalysisResult carries the diagnostics of one fixpoint run.
type AnalysisResult struct{ Diagnostics []Diagnostic }

// Analyzer runs the definite-initialization and narrowing fixpoint over a
// CFG; blocks without incoming facts are unreachable and contribute
// nothing.
type Analyzer struct{}

// CFG is the kernel control-flow graph: blocks by ID plus directed edges.
type CFG struct {
	blocks map[BlockID]Block
	edges  map[BlockID][]BlockID
}

// NewCFG indexes the given blocks; the entry block is expected at ID 1.
func NewCFG(blocks ...Block) *CFG {
	cfg := &CFG{blocks: make(map[BlockID]Block), edges: make(map[BlockID][]BlockID)}
	for _, block := range blocks {
		cfg.blocks[block.ID] = block
	}
	return cfg
}

// AddEdge wires a control-flow edge between two blocks.
func (cfg *CFG) AddEdge(from, to BlockID) {
	cfg.edges[from] = append(cfg.edges[from], to)
}

// Successors returns the reachable blocks wired after block.
func (cfg *CFG) Successors(block BlockID) []Block {
	ids := cfg.edges[block]
	result := make([]Block, 0, len(ids))
	for _, id := range ids {
		if target, ok := cfg.blocks[id]; ok {
			result = append(result, target)
		}
	}
	return result
}

// FallOffEnd reports whether the body can complete without a Return: a
// path from the entry block (ID 1) reaches a block that has no successors
// and does not end with a Return operation (RFC-001 §13.18 rule 18,
// §8.2.6 D-6). A Return-terminated block never continues the flow, so its
// successors - the disconnected continuations after the return - are not
// followed, and blocks unreachable from the entry contribute nothing: an
// infinite loop with no exit keeps the body from falling off (Go
// reference: `for {}` satisfies the return requirement).
func FallOffEnd(cfg *CFG) bool {
	seen := map[BlockID]bool{1: true}
	queue := []BlockID{1}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		block, ok := cfg.blocks[id]
		if !ok {
			continue
		}
		if endsWithReturn(block) {
			continue
		}
		if len(cfg.edges[id]) == 0 {
			return true
		}
		for _, next := range cfg.edges[id] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func endsWithReturn(block Block) bool {
	n := len(block.Operations)
	return n > 0 && block.Operations[n-1].Kind == ReturnOperation
}

// FactSet is the per-binding fact lattice shared by both fact dimensions.
type FactSet map[BindingID]BindingState

// NewFactSet returns an empty fact set.
func NewFactSet() FactSet { return make(FactSet) }

// State reports the recorded state; the zero value is Uninitialized.
func (facts FactSet) State(binding BindingID) BindingState { return facts[binding] }

// Assign records the initialized state.
func (facts FactSet) Assign(binding BindingID) { facts[binding] = Initialized }

func (Analyzer) Analyze(cfg *CFG) AnalysisResult {
	in := make(map[BlockID]FactSet)
	in[1] = NewFactSet()
	// Non-nil narrowing (story 05): a second fact dimension tracked in the
	// same fixpoint. Transfer functions stay constant-per-binding, so the
	// chaotic iteration remains monotone over a finite lattice and
	// converges on loop back edges.
	nonNil := make(map[BlockID]FactSet)
	nonNil[1] = NewFactSet()
	changed := true
	result := AnalysisResult{}
	reported := make(map[BlockID]map[BindingID]bool)
	reportedUnsafe := make(map[BlockID]map[BindingID]bool)
	// Cascade suppression (CONTRACTS §2, decision 4): per (block, binding),
	// the position of the block's ReadBeforeInitialization primary in the
	// result, plus derefs that fired before their block's read — they wait
	// to fold into the primary instead of standing alone.
	rbInitAt := make(map[BlockID]map[BindingID]int)
	type pendingDeref struct {
		binding BindingID
		diag    Diagnostic
	}
	pendingUnsafe := make(map[BlockID][]pendingDeref)
	for changed {
		changed = false
		for id, block := range cfg.blocks {
			facts, ok := in[id]
			if !ok {
				continue
			}
			out := cloneFacts(facts)
			nonNilOut := cloneFacts(nonNil[id])
			for _, op := range block.Operations {
				switch op.Kind {
				case DeclareOperation:
					out[op.Binding] = Uninitialized
					nonNilOut[op.Binding] = Uninitialized
				case AssignOperation:
					out.Assign(op.Binding)
					// RFC-002 §25: assignment invalidates narrowing; the
					// experimental layer has no RHS types to re-establish.
					nonNilOut[op.Binding] = Uninitialized
				case ReadOperation:
					if out.State(op.Binding) != Initialized {
						if reported[id] == nil {
							reported[id] = make(map[BindingID]bool)
						}
						if !reported[id][op.Binding] {
							result.Diagnostics = append(result.Diagnostics, NewDiagnostic(ReadBeforeInitializationDescriptor, op.Binding, SourceSpan{}))
							reported[id][op.Binding] = true
							if rbInitAt[id] == nil {
								rbInitAt[id] = make(map[BindingID]int)
							}
							primary := len(result.Diagnostics) - 1
							rbInitAt[id][op.Binding] = primary
							for i, p := range pendingUnsafe[id] {
								if p.binding == op.Binding {
									result.Diagnostics[primary].Related = append(result.Diagnostics[primary].Related, p.diag)
									pendingUnsafe[id] = append(pendingUnsafe[id][:i], pendingUnsafe[id][i+1:]...)
									break
								}
							}
						}
					}
				case AssumeOperation:
					nonNilOut.Assign(op.Binding)
				case DerefOperation:
					if nonNilOut.State(op.Binding) != Initialized {
						if reportedUnsafe[id] == nil {
							reportedUnsafe[id] = make(map[BindingID]bool)
						}
						if !reportedUnsafe[id][op.Binding] {
							uma := NewDiagnostic(UnsafeMemberAccessDescriptor, op.Binding, SourceSpan{})
							reportedUnsafe[id][op.Binding] = true
							if primary, coFired := rbInitAt[id][op.Binding]; coFired {
								// The read already fired in this block: the
								// deref nests into it (CONTRACTS §2).
								result.Diagnostics[primary].Related = append(result.Diagnostics[primary].Related, uma)
							} else {
								// Deferred: the block's read may still fire
								// later and fold it in; block end publishes
								// leftovers standalone.
								pendingUnsafe[id] = append(pendingUnsafe[id], pendingDeref{binding: op.Binding, diag: uma})
							}
						}
					}
				case CallOperation:
					// RFC-003 §85: the call of a mutating closure invalidates
					// its captured bindings' narrowing; §84 keeps the
					// initialization dimension untouched.
					for _, mutated := range op.Mutates {
						nonNilOut[mutated] = Uninitialized
					}
				}
			}
			for _, p := range pendingUnsafe[id] {
				result.Diagnostics = append(result.Diagnostics, p.diag)
			}
			pendingUnsafe[id] = nil
			for _, next := range cfg.edges[id] {
				if old, exists := in[next]; !exists {
					in[next] = cloneFacts(out)
					changed = true
				} else if mergeFacts(old, out) {
					changed = true
				}
				if _, exists := nonNil[next]; !exists {
					nonNil[next] = cloneFacts(nonNilOut)
					changed = true
				} else if mergeFacts(nonNil[next], nonNilOut) {
					changed = true
				}
			}
		}
	}
	return result
}
func cloneFacts(in FactSet) FactSet {
	out := NewFactSet()
	for k, v := range in {
		out[k] = v
	}
	return out
}
func mergeFacts(dst, incoming FactSet) bool {
	changed := false
	for k, v := range dst {
		if v == Initialized && incoming.State(k) != Initialized {
			dst[k] = Uninitialized
			changed = true
		}
	}
	return changed
}
