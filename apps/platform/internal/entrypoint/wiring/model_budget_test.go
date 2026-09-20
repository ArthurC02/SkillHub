package wiring

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/modelbudget"
)

func TestEveryModelCallOnTheRosterCanActuallyBeRetimed(t *testing.T) {
	seen := map[string]bool{}
	for _, e := range NewModelBudgets(nil).Endpoints {
		if e.Kind == "" {
			t.Errorf("an endpoint with a %s deadline has no kind; an operator cannot name it", e.Deadline)
		}
		if seen[e.Kind] {
			t.Errorf("%q is on the roster twice; two deadlines would share one stored value", e.Kind)
		}
		seen[e.Kind] = true
		if e.Ceiling() < modelbudget.MinSeconds {
			t.Errorf("%q allows at most %d seconds, so nothing can be set for it; its %s deadline "+
				"leaves no room above the %s margin", e.Kind, e.Ceiling(), e.Deadline, modelbudget.Margin)
		}
	}
	if len(seen) == 0 {
		t.Fatal("the roster is empty, so the settings page would offer nothing")
	}
}
