package creation

import (
	"fmt"
	"testing"
)

func TestADiagramReadingNeedsAtLeastOneNodeAndAtMostTheItemLimit(t *testing.T) {
	nodes := func(n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = fmt.Sprintf("node %d", i)
		}
		return out
	}
	for _, tc := range []struct {
		name  string
		nodes []string
		valid bool
	}{
		{"no node", nil, false},
		{"one node", nodes(1), true},
		{"at the item limit", nodes(maxDiagramItems), true},
		{"one past the item limit", nodes(maxDiagramItems + 1), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := validDiagramDecomposition(&DiagramDecomposition{Nodes: tc.nodes}); got != tc.valid {
				t.Errorf("validDiagramDecomposition = %v, want %v", got, tc.valid)
			}
			if got := validDiagramInterpretation(&DiagramInterpretation{Nodes: tc.nodes}); got != tc.valid {
				t.Errorf("validDiagramInterpretation = %v, want %v", got, tc.valid)
			}
		})
	}
}

func TestADiagramReadingMayHaveNoConditionsBranchesOrUncertainties(t *testing.T) {
	if !validDiagramDecomposition(&DiagramDecomposition{Nodes: []string{"start"}}) {
		t.Error("a decomposition with only nodes was refused")
	}
}
