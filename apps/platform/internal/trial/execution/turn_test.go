package run

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

var turnEpoch = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

func lineRun(id, workspace byte, status gen.RunStatus, createdAfter time.Duration) gen.Run {
	return gen.Run{
		ID:          pgtype.UUID{Bytes: [16]byte{15: id}, Valid: true},
		WorkspaceID: pgtype.UUID{Bytes: [16]byte{15: workspace}, Valid: true},
		Status:      status,
		CreatedAt:   pgtype.Timestamptz{Time: turnEpoch.Add(createdAfter), Valid: true},
	}
}

func aheadIDs(line runLine, of gen.Run) []byte {
	var ids []byte
	for _, r := range line.ahead(of) {
		ids = append(ids, r.ID.Bytes[15])
	}
	return ids
}

func TestAWorkspaceHoldingFewerSandboxesGoesFirstEvenWhenItQueuedLater(t *testing.T) {
	holding := lineRun(1, 0xA, gen.RunStatusRunning, 0)
	earlyFromBusy := lineRun(2, 0xA, gen.RunStatusQueued, time.Minute)
	lateFromIdle := lineRun(3, 0xB, gen.RunStatusQueued, 2*time.Minute)
	line := lineOf([]gen.Run{holding, earlyFromBusy, lateFromIdle})

	if got := aheadIDs(line, earlyFromBusy); len(got) != 1 || got[0] != 3 {
		t.Errorf("ahead of the busy workspace's run = %v, want [3]", got)
	}
	if got := aheadIDs(line, lateFromIdle); len(got) != 0 {
		t.Errorf("ahead of the idle workspace's run = %v, want none", got)
	}
}

func TestAmongEquallyServedWorkspacesTheRunThatQueuedFirstGoesFirst(t *testing.T) {
	older := lineRun(9, 0xA, gen.RunStatusQueued, time.Minute)
	newer := lineRun(1, 0xB, gen.RunStatusQueued, time.Minute+time.Nanosecond)
	line := lineOf([]gen.Run{older, newer})

	if got := aheadIDs(line, newer); len(got) != 1 || got[0] != 9 {
		t.Errorf("ahead of the newer run = %v, want [9]", got)
	}
	if got := aheadIDs(line, older); len(got) != 0 {
		t.Errorf("ahead of the older run = %v, want none", got)
	}
}

func TestRunsQueuedAtTheSameInstantTakeTurnsInIDOrder(t *testing.T) {
	low := lineRun(1, 0xA, gen.RunStatusQueued, 0)
	high := lineRun(2, 0xB, gen.RunStatusQueued, 0)
	line := lineOf([]gen.Run{high, low})

	if got := aheadIDs(line, high); len(got) != 1 || got[0] != 1 {
		t.Errorf("ahead of id 2 = %v, want [1]", got)
	}
	if got := aheadIDs(line, low); len(got) != 0 {
		t.Errorf("ahead of id 1 = %v, want none", got)
	}
}

func TestEveryDispatchedStateCountsAsASandboxHeldAndNeverStandsInLine(t *testing.T) {
	for _, status := range []gen.RunStatus{
		gen.RunStatusProvisioning, gen.RunStatusPreparing, gen.RunStatusRunning, gen.RunStatusEvaluating,
	} {
		t.Run(string(status), func(t *testing.T) {
			dispatched := lineRun(1, 0xA, status, 0)
			fromThatWorkspace := lineRun(2, 0xA, gen.RunStatusQueued, 0)
			fromAnother := lineRun(3, 0xB, gen.RunStatusQueued, time.Hour)
			line := lineOf([]gen.Run{dispatched, fromThatWorkspace, fromAnother})

			if got := aheadIDs(line, fromThatWorkspace); len(got) != 1 || got[0] != 3 {
				t.Errorf("ahead of the run whose workspace holds a %s run = %v, want [3]", status, got)
			}
		})
	}
}

func TestARunWhoseCancellationWasRequestedDoesNotHoldAPlaceInLine(t *testing.T) {
	cancelled := lineRun(1, 0xA, gen.RunStatusQueued, 0)
	cancelled.CancelRequestedAt = pgtype.Timestamptz{Time: turnEpoch, Valid: true}
	waiting := lineRun(2, 0xB, gen.RunStatusQueued, time.Minute)
	line := lineOf([]gen.Run{cancelled, waiting})

	if got := aheadIDs(line, waiting); len(got) != 0 {
		t.Errorf("ahead of the waiting run = %v, want none: the cancelled run gave up its place", got)
	}
}
