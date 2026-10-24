package semantic

// PackageInitGroup is one initialization step (RFC-024 §6.6): the package
// bindings declared by one statement and the package bindings its
// initializer transitively references through same-package code. A
// multi-variable declaration forms one group (§6.6: one initialization
// step); Bindings keeps the declaration order.
type PackageInitGroup struct {
	Bindings []BindingID
	Deps     []BindingID
}

// PackageInitGraph orders package-variable initialization groups (RFC-024
// §6.6) and reports dependency cycles (§8.2). Groups carry the canonical
// declaration order of the slice they are built from; the graph itself is
// pure - the checker owns reference collection, the graph owns ordering
// and cycle verdicts.
type PackageInitGraph struct {
	groups []PackageInitGroup
	// ownerOf maps a package binding to the index of its declaring group.
	ownerOf map[BindingID]int
}

// NewPackageInitGraph indexes the groups; duplicate bindings across groups
// are the caller's contract violation and index to the last owner.
func NewPackageInitGraph(groups []PackageInitGroup) *PackageInitGraph {
	g := &PackageInitGraph{groups: groups, ownerOf: make(map[BindingID]int)}
	for i := range groups {
		for _, binding := range groups[i].Bindings {
			g.ownerOf[binding] = i
		}
	}
	return g
}

// ReadyOrder returns the §6.6 stepwise order: repeatedly the earliest group
// in canonical declaration order whose dependencies are all initialized.
// Dependencies on bindings outside any group (locals, foreign names) do
// not block. ok is false when a cycle leaves groups uninitializeable; the
// returned order then covers the initializeable prefix.
func (g *PackageInitGraph) ReadyOrder() (order []int, ok bool) {
	initialized := make([]bool, len(g.groups))
	order = make([]int, 0, len(g.groups))
	for len(order) < len(g.groups) {
		progress := false
		for i := range g.groups {
			if initialized[i] || !g.ready(i, initialized) {
				continue
			}
			initialized[i] = true
			order = append(order, i)
			progress = true
			break
		}
		if !progress {
			return order, false
		}
	}
	return order, true
}

// ready reports whether every package-state dependency of the group is
// initialized (§6.6: a group is ready when every package variable on which
// it depends has completed initialization).
func (g *PackageInitGraph) ready(index int, initialized []bool) bool {
	for _, dep := range g.groups[index].Deps {
		if owner, ok := g.ownerOf[dep]; ok && !initialized[owner] {
			return false
		}
	}
	return true
}

// ShortestCycle returns the group indices of the shortest dependency cycle
// (§8.2 shows the chain A → B → A; the chain repeats its head when
// rendered). Ties break by the lowest start index, then by scan order -
// deterministic for the §6.12.18 RFC-010 reproducibility contract. Empty
// when the graph is acyclic.
func (g *PackageInitGraph) ShortestCycle() []int {
	var best []int
	for start := range g.groups {
		cycle := g.shortestThrough(start)
		if cycle == nil {
			continue
		}
		if best == nil || len(cycle) < len(best) {
			best = cycle
		}
	}
	return best
}

// shortestThrough returns the shortest cycle that returns to start, found
// by breadth-first search over the depends-on edges; nil when no cycle
// through start exists.
func (g *PackageInitGraph) shortestThrough(start int) []int {
	parent := make([]int, len(g.groups))
	seen := make([]bool, len(g.groups))
	seen[start] = true
	queue := []int{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, dep := range g.groups[current].Deps {
			next, ok := g.ownerOf[dep]
			if !ok {
				continue
			}
			if next == start {
				// Reconstruct the walk start → … → current → start.
				chain := []int{current}
				for at := current; at != start; at = parent[at] {
					chain = append(chain, parent[at])
				}
				// Reverse into start-first order.
				for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
					chain[i], chain[j] = chain[j], chain[i]
				}
				return chain
			}
			if seen[next] {
				continue
			}
			seen[next] = true
			parent[next] = current
			queue = append(queue, next)
		}
	}
	return nil
}
