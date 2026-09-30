package run

import (
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestRunActivityClassificationCoversEveryLifecycleState(t *testing.T) {
	tests := []struct {
		status gen.RunStatus
		want   string
	}{
		{gen.RunStatusQueued, "in_progress"},
		{gen.RunStatusProvisioning, "in_progress"},
		{gen.RunStatusPreparing, "in_progress"},
		{gen.RunStatusRunning, "in_progress"},
		{gen.RunStatusEvaluating, "in_progress"},
		{gen.RunStatusSucceeded, "recent"},
		{gen.RunStatusCancelled, "recent"},
		{gen.RunStatusFailed, "needs_attention"},
		{gen.RunStatusTimedOut, "needs_attention"},
		{"future_state", "needs_attention"},
	}
	for _, test := range tests {
		t.Run(string(test.status), func(t *testing.T) {
			got, label := classifyRunActivity(test.status)
			if got != test.want || label == "" {
				t.Fatalf("classification = %q, label = %q, want %q and a label", got, label, test.want)
			}
		})
	}
}
