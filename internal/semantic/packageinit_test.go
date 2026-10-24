package semantic

import (
	"reflect"
	"testing"
)

// RFC-024 §6.6: a group is ready when every package variable it depends on
// completed initialization; the compiler repeatedly initializes the earliest
// ready group in canonical declaration order - a forward textual dependency
// is legal because the dependency graph reorders it (§6.2 motivation).
func TestPackageInitGraphReadyOrderReordersForwardDependency(t *testing.T) {
	// var endpoint = Build(config)  -> group 0, depends on config (group 1)
	// var config = Load()           -> group 1, no deps
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}, Deps: []BindingID{2}},
		{Bindings: []BindingID{2}},
	})
	order, ok := graph.ReadyOrder()
	if !ok {
		t.Fatal("ReadyOrder reported a cycle for a legal forward dependency")
	}
	if !reflect.DeepEqual(order, []int{1, 0}) {
		t.Fatalf("order = %v, want [1 0]", order)
	}
}

// §6.6: a multi-variable group is one initialization step - its bindings
// share one order position, and a dependency on any of them blocks through
// the whole group.
func TestPackageInitGraphMultiNameGroupIsOneStep(t *testing.T) {
	// var a, b = MakePair() -> group 0 (bindings 1, 2)
	// var c = a             -> group 1, depends on a
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1, 2}},
		{Bindings: []BindingID{3}, Deps: []BindingID{1}},
	})
	order, ok := graph.ReadyOrder()
	if !ok {
		t.Fatal("ReadyOrder reported a cycle for a legal group dependency")
	}
	if !reflect.DeepEqual(order, []int{0, 1}) {
		t.Fatalf("order = %v, want [0 1]", order)
	}
}

// §6.6: source order is the deterministic tie-break between ready groups.
func TestPackageInitGraphTieBreakFollowsCanonicalOrder(t *testing.T) {
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}},
		{Bindings: []BindingID{2}},
		{Bindings: []BindingID{3}},
	})
	order, ok := graph.ReadyOrder()
	if !ok {
		t.Fatal("ReadyOrder reported a cycle for independent groups")
	}
	if !reflect.DeepEqual(order, []int{0, 1, 2}) {
		t.Fatalf("order = %v, want [0 1 2]", order)
	}
}

// §6.4/§6.6: references to bindings that are not package state (locals,
// foreign names) do not create dependencies.
func TestPackageInitGraphIgnoresNonPackageDeps(t *testing.T) {
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}, Deps: []BindingID{99, 100}},
	})
	if _, ok := graph.ReadyOrder(); !ok {
		t.Fatal("ReadyOrder reported a cycle over non-package references")
	}
	if cycle := graph.ShortestCycle(); len(cycle) != 0 {
		t.Fatalf("ShortestCycle = %v, want empty", cycle)
	}
}

// §8.2: an initialization cycle is a compile-time error; the diagnostic
// shows the shortest found dependency cycle (A → B → A).
func TestPackageInitGraphShortestCycle(t *testing.T) {
	// var a = b -> group 0 depends on b (group 1)
	// var b = a -> group 1 depends on a (group 0)
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}, Deps: []BindingID{2}},
		{Bindings: []BindingID{2}, Deps: []BindingID{1}},
	})
	if _, ok := graph.ReadyOrder(); ok {
		t.Fatal("ReadyOrder accepted a dependency cycle")
	}
	if cycle := graph.ShortestCycle(); !reflect.DeepEqual(cycle, []int{0, 1}) {
		t.Fatalf("ShortestCycle = %v, want [0 1]", cycle)
	}
}

// §8.2: a self-referential initializer (`var a = a`) is a one-group cycle.
func TestPackageInitGraphSelfCycle(t *testing.T) {
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}, Deps: []BindingID{1}},
	})
	if _, ok := graph.ReadyOrder(); ok {
		t.Fatal("ReadyOrder accepted a self cycle")
	}
	if cycle := graph.ShortestCycle(); !reflect.DeepEqual(cycle, []int{0}) {
		t.Fatalf("ShortestCycle = %v, want [0]", cycle)
	}
}

// §8.2: among several cycles the shortest found one is reported, and the
// scan stays deterministic (lowest start index wins ties).
func TestPackageInitGraphShortestCyclePrefersShortest(t *testing.T) {
	// group 0 -> 1 -> 2 -> 0 (length 3), group 3 self-cycle (length 1)
	graph := NewPackageInitGraph([]PackageInitGroup{
		{Bindings: []BindingID{1}, Deps: []BindingID{2}},
		{Bindings: []BindingID{2}, Deps: []BindingID{3}},
		{Bindings: []BindingID{3}, Deps: []BindingID{1}},
		{Bindings: []BindingID{4}, Deps: []BindingID{4}},
	})
	if cycle := graph.ShortestCycle(); !reflect.DeepEqual(cycle, []int{3}) {
		t.Fatalf("ShortestCycle = %v, want [3]", cycle)
	}
}
