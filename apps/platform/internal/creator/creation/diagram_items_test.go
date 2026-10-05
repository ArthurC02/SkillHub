package creation

import (
	"fmt"
	"testing"
)

func diagramItems(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("item %d", i)
	}
	return out
}

func TestADiagramReadingKeepsEachListWithinItsItemLimit(t *testing.T) {
	start := []string{"start"}
	for _, tc := range []struct {
		name                        string
		nodes, conditions, branches []string
		valid                       bool
	}{
		{"no node", nil, nil, nil, false},
		{"one node and nothing else", start, nil, nil, true},
		{"64 nodes", diagramItems(64), nil, nil, true},
		{"65 nodes", diagramItems(65), nil, nil, false},
		{"64 conditions", start, diagramItems(64), nil, true},
		{"65 conditions", start, diagramItems(65), nil, false},
		{"128 branches", start, nil, diagramItems(128), true},
		{"129 branches", start, nil, diagramItems(129), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decomposition := &DiagramDecomposition{Nodes: tc.nodes, Conditions: tc.conditions, Branches: tc.branches}
			if got := validDiagramDecomposition(decomposition); got != tc.valid {
				t.Errorf("validDiagramDecomposition = %v, want %v", got, tc.valid)
			}
			interpretation := &DiagramInterpretation{Nodes: tc.nodes, Conditions: tc.conditions, Branches: tc.branches}
			if got := validDiagramInterpretation(interpretation); got != tc.valid {
				t.Errorf("validDiagramInterpretation = %v, want %v", got, tc.valid)
			}
		})
	}
}

func TestADiagramDecompositionAsksAtMost64Questions(t *testing.T) {
	for _, tc := range []struct {
		questions int
		valid     bool
	}{{64, true}, {65, false}} {
		t.Run(fmt.Sprint(tc.questions), func(t *testing.T) {
			d := &DiagramDecomposition{Nodes: []string{"start"}, Uncertainties: diagramItems(tc.questions)}
			if got := validDiagramDecomposition(d); got != tc.valid {
				t.Errorf("validDiagramDecomposition with %d questions = %v, want %v", tc.questions, got, tc.valid)
			}
		})
	}
}
