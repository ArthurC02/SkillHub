package dockerdrv

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/sandbox/internal/sandbox"
)

func namesSetBy(lines []string) map[string]bool {
	set := map[string]bool{}
	for _, line := range lines {
		if name, _, ok := strings.Cut(line, "="); ok {
			set[name] = true
		}
	}
	return set
}

func TestTheNamesDeclaredAreTheNamesTheGrantActuallySets(t *testing.T) {
	withGrant := env(sandbox.RunRequest{
		ModelGateway: &sandbox.ModelGatewayGrant{
			BaseURL: "https://gateway.test", VirtualKey: "sk-not-a-real-key",
		},
	})
	without := env(sandbox.RunRequest{})

	before, after := namesSetBy(without), namesSetBy(withGrant)
	added := []string{}
	for name := range after {
		if !before[name] {
			added = append(added, name)
		}
	}
	sort.Strings(added)

	declared := append([]string{}, (&Driver{}).InjectsFromGrant()...)
	sort.Strings(declared)

	if !slices.Equal(added, declared) {
		t.Errorf("the workload is given %v but this provider declares %v; the pre-run summary "+
			"shows the declaration, so a difference is a false claim about secrets", added, declared)
	}
	if len(added) == 0 {
		t.Fatal("the grant added no variable at all; this test would pass on any declaration")
	}
}
