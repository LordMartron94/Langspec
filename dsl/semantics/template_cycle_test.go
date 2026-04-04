package semantics

import "testing"

func TestTemplateExpansionCyclePathDetectsSelfLoop(t *testing.T) {
	graph := map[string][]string{
		"A": {"A"},
	}
	c := templateExpansionCyclePath(graph)
	if len(c) == 0 {
		t.Fatal("expected cycle path for A -> A")
	}
}

func TestTemplateExpansionCyclePathAcyclic(t *testing.T) {
	graph := map[string][]string{
		"A": {"B"},
		"B": {},
	}
	c := templateExpansionCyclePath(graph)
	if len(c) != 0 {
		t.Fatalf("expected no cycle, got %v", c)
	}
}
