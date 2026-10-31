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

func TestTheQueuePositionCountsTheRunsWhoseTurnComesFirst(t *testing.T) {
	holding := lineRun(1, 0xA, gen.RunStatusRunning, 0)
	earlyFromBusy := lineRun(2, 0xA, gen.RunStatusQueued, time.Minute)
	lateFromIdle := lineRun(3, 0xB, gen.RunStatusQueued, 2*time.Minute)
	latest := lineRun(4, 0xC, gen.RunStatusQueued, 3*time.Minute)
	unfinished := []gen.Run{holding, earlyFromBusy, lateFromIdle, latest}

	for _, tc := range []struct {
		name string
		run  gen.Run
		want int
	}{
		{"first in line", lateFromIdle, 0},
		{"behind the idle workspace", latest, 1},
		{"its workspace already holds a sandbox", earlyFromBusy, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, counted := queuePosition(unfinished, tc.run.ID)
			if !counted || got != tc.want {
				t.Errorf("queuePosition = %d, %v; want %d, true", got, counted, tc.want)
			}
		})
	}
}

func TestTheQueuePositionIsNotCountedForARunThatDoesNotStandInLine(t *testing.T) {
	running := lineRun(1, 0xA, gen.RunStatusRunning, 0)
	cancelled := lineRun(2, 0xB, gen.RunStatusQueued, 0)
	cancelled.CancelRequestedAt = pgtype.Timestamptz{Time: turnEpoch, Valid: true}
	waiting := lineRun(3, 0xC, gen.RunStatusQueued, time.Minute)
	unfinished := []gen.Run{running, cancelled, waiting}

	for _, tc := range []struct {
		name string
		id   pgtype.UUID
	}{
		{"dispatched", running.ID},
		{"cancellation requested", cancelled.ID},
		{"outside the unfinished runs read", lineRun(9, 0xD, gen.RunStatusQueued, 0).ID},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got, counted := queuePosition(unfinished, tc.id); counted {
				t.Errorf("queuePosition = %d, counted; want not counted", got)
			}
		})
	}
}

func TestTheQueuePositionIsNotCountedOnceTheReadFillsItsWindow(t *testing.T) {
	for _, tc := range []struct {
		name    string
		size    int
		counted bool
	}{
		{"one short of the window", turnWindow - 1, true},
		{"the window is full", turnWindow, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			unfinished := make([]gen.Run, tc.size)
			for i := range unfinished {
				unfinished[i] = gen.Run{
					ID:          pgtype.UUID{Bytes: [16]byte{0: byte(i >> 8), 1: byte(i)}, Valid: true},
					WorkspaceID: pgtype.UUID{Bytes: [16]byte{0: byte(i >> 8), 1: byte(i)}, Valid: true},
					Status:      gen.RunStatusQueued,
					CreatedAt:   pgtype.Timestamptz{Time: turnEpoch.Add(time.Duration(i) * time.Second), Valid: true},
				}
			}
			last := unfinished[tc.size-1]
			got, counted := queuePosition(unfinished, last.ID)
			if counted != tc.counted {
				t.Fatalf("queuePosition = %d, %v; want counted %v", got, counted, tc.counted)
			}
			if counted && got != tc.size-1 {
				t.Errorf("queuePosition = %d, want %d", got, tc.size-1)
			}
		})
	}
}
