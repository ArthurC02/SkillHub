package creation

import (
	"strings"
	"testing"
)

func TestValidDiagramTextRuneBoundaries(t *testing.T) {
	t.Parallel()
	max := strings.Repeat("界", 2000)
	over := max + "界"
	for name, tc := range map[string]struct {
		value string
		valid bool
	}{
		"blank": {value: " \n\t ", valid: false},
		"max":   {value: max, valid: true},
		"over":  {value: over, valid: false},
	} {
		t.Run(name, func(t *testing.T) {
			if got := validDiagramDescription(tc.value); got != tc.valid {
				t.Fatalf("validDiagramDescription(%q) = %v, want %v", name, got, tc.valid)
			}
			if got := validDiagramAnswer(tc.value); got != tc.valid {
				t.Fatalf("validDiagramAnswer(%q) = %v, want %v", name, got, tc.valid)
			}
			if got := validDiagramInterpretation(&DiagramInterpretation{Nodes: []string{tc.value}}); got != tc.valid {
				t.Fatalf("validDiagramInterpretation(%q) = %v, want %v", name, got, tc.valid)
			}
		})
	}
}
