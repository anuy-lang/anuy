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
	ID BlockID
}

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
