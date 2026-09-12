// Package semantic contains experimental compiler dataflow primitives.
package semantic

type BindingID uint32
type BlockID uint32

type BindingState uint8

const (
	Uninitialized BindingState = iota
	Initialized
)

type Block struct {
	ID         BlockID
	Operations []Operation
}

type OperationKind uint8

const (
	DeclareOperation OperationKind = iota
	AssignOperation
	ReadOperation
)

type Operation struct {
	Kind    OperationKind
	Binding BindingID
}

func Declare(binding BindingID) Operation { return Operation{Kind: DeclareOperation, Binding: binding} }
func Assign(binding BindingID) Operation  { return Operation{Kind: AssignOperation, Binding: binding} }
func Read(binding BindingID) Operation    { return Operation{Kind: ReadOperation, Binding: binding} }

type DiagnosticCategory string

const (
	ReadBeforeInitialization   DiagnosticCategory = "ReadBeforeInitialization"
	PackageInitializerRequired DiagnosticCategory = "PackageInitializerRequired"
)

type SourceSpan struct {
	File       string
	Start, End int
}

type Diagnostic struct {
	Category DiagnosticCategory
	Binding  BindingID
	Span     SourceSpan
}

func NewDiagnostic(category DiagnosticCategory, binding BindingID, span SourceSpan) Diagnostic {
	return Diagnostic{Category: category, Binding: binding, Span: span}
}

func ValidatePackageBinding(hasInitializer bool) *Diagnostic {
	if hasInitializer {
		return nil
	}
	return &Diagnostic{Category: PackageInitializerRequired}
}

type AnalysisResult struct{ Diagnostics []Diagnostic }
type Analyzer struct{}

type CFG struct {
	blocks map[BlockID]Block
	edges  map[BlockID][]BlockID
}

func NewCFG(blocks ...Block) *CFG {
	cfg := &CFG{blocks: make(map[BlockID]Block), edges: make(map[BlockID][]BlockID)}
	for _, block := range blocks {
		cfg.blocks[block.ID] = block
	}
	return cfg
}

func (cfg *CFG) AddEdge(from, to BlockID) {
	cfg.edges[from] = append(cfg.edges[from], to)
}

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

type FactSet map[BindingID]BindingState

func NewFactSet() FactSet { return make(FactSet) }

func (facts FactSet) State(binding BindingID) BindingState { return facts[binding] }

func (facts FactSet) Assign(binding BindingID) { facts[binding] = Initialized }

func (Analyzer) Analyze(cfg *CFG) AnalysisResult {
	in := make(map[BlockID]FactSet)
	in[1] = NewFactSet()
	changed := true
	result := AnalysisResult{}
	reported := make(map[BlockID]map[BindingID]bool)
	for changed {
		changed = false
		for id, block := range cfg.blocks {
			facts, ok := in[id]
			if !ok {
				continue
			}
			out := cloneFacts(facts)
			for _, op := range block.Operations {
				switch op.Kind {
				case DeclareOperation:
					out[op.Binding] = Uninitialized
				case AssignOperation:
					out.Assign(op.Binding)
				case ReadOperation:
					if out.State(op.Binding) != Initialized {
						if reported[id] == nil {
							reported[id] = make(map[BindingID]bool)
						}
						if !reported[id][op.Binding] {
							result.Diagnostics = append(result.Diagnostics, Diagnostic{Category: ReadBeforeInitialization, Binding: op.Binding})
							reported[id][op.Binding] = true
						}
					}
				}
			}
			for _, next := range cfg.edges[id] {
				if old, exists := in[next]; !exists {
					in[next] = cloneFacts(out)
					changed = true
				} else if mergeFacts(old, out) {
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
