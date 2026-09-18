package semantic

import "testing"

func TestCFGSuccessorsFollowDeclaredEdges(t *testing.T) {
	cfg := NewCFG(
		Block{ID: 1},
		Block{ID: 2},
		Block{ID: 3},
	)
	cfg.AddEdge(1, 2)
	cfg.AddEdge(1, 3)

	got := cfg.Successors(1)
	if len(got) != 2 || got[0].ID != 2 || got[1].ID != 3 {
		t.Fatalf("Successors(1) = %#v, want blocks 2 and 3", got)
	}
}

func TestFactSetTracksBindingInitialization(t *testing.T) {
	facts := NewFactSet()
	binding := BindingID(7)

	if state := facts.State(binding); state != Uninitialized {
		t.Fatalf("new binding state = %v, want %v", state, Uninitialized)
	}
	facts.Assign(binding)
	if state := facts.State(binding); state != Initialized {
		t.Fatalf("assigned binding state = %v, want %v", state, Initialized)
	}
}
