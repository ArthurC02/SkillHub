package localdrv

import (
	"slices"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func TestARunVariableReplacesTheHostValueInsteadOfJoiningIt(t *testing.T) {
	t.Setenv("HOME", "host-home")

	var homes []string
	for _, kv := range env(sandbox.RunRequest{}, "work", "out") {
		if strings.HasPrefix(kv, "HOME=") {
			homes = append(homes, kv)
		}
	}
	if !slices.Equal(homes, []string{"HOME=work"}) {
		t.Fatalf("HOME entries = %v, want only the run's own HOME=work", homes)
	}
}
