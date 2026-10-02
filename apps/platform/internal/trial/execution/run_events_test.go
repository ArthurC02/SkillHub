package run

import (
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/messaging/outbox"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestEveryRunStatusMapsIntoTheClosedSet(t *testing.T) {
	statuses := []gen.RunStatus{
		gen.RunStatusQueued, gen.RunStatusProvisioning, gen.RunStatusPreparing,
		gen.RunStatusRunning, gen.RunStatusEvaluating, gen.RunStatusSucceeded,
		gen.RunStatusFailed, gen.RunStatusCancelled, gen.RunStatusTimedOut,
	}
	for _, status := range statuses {
		event := StatusChanged{RunStatusChanged: outbox.RunStatusChanged{ToStatus: string(status)}}.eventType()
		if !slices.Contains(outbox.EventTypes, event) {
			t.Errorf("run status %q maps to %q, which is not in the closed set", status, event)
		}
	}
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusCleaned, gen.RunCleanupStatusFailed} {
		if event, ok := cleanupEvents[status]; !ok || !slices.Contains(outbox.EventTypes, event) {
			t.Errorf("cleanup status %q maps to %q, which is not in the closed set", status, event)
		}
	}
}

func TestOnlySettledCleanupStatusesHaveAnEvent(t *testing.T) {
	for _, status := range []gen.RunCleanupStatus{gen.RunCleanupStatusPending, gen.RunCleanupStatusCleaningUp} {
		if event, ok := cleanupEvents[status]; ok {
			t.Errorf("cleanup status %q produced %q, want no event", status, event)
		}
	}
}
