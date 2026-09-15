package run

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestEvaluationRunCarriesTheTerminalVerdict(t *testing.T) {
	for _, status := range AllStatuses {
		t.Run(string(status), func(t *testing.T) {
			facts := evaluationRun(gen.Run{Status: status})
			if facts.Status != string(status) || facts.Terminal != IsTerminal(status) {
				t.Fatalf("status %s: got status %q, terminal %v; want terminal %v", status, facts.Status, facts.Terminal, IsTerminal(status))
			}
		})
	}
}
