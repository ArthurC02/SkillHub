package run

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func TestAnAbsentQueueDoesNotReachTheRunServiceLookingPresent(t *testing.T) {
	var unconfigured *river.Client[pgx.Tx]

	if q := NewRunQueue(unconfigured); q != nil {
		t.Error("an unconfigured client arrived as a non-nil RunQueue; every `Queue == nil` guard in the " +
			"run service now passes and the first enqueue panics")
	}
}

func TestQueuedWorkNamesTheRunAndTheWorkspaceItBelongsTo(t *testing.T) {
	run := gen.Run{
		ID:          pgtype.UUID{Bytes: [16]byte{0: 1}, Valid: true},
		WorkspaceID: pgtype.UUID{Bytes: [16]byte{0: 2}, Valid: true},
	}
	wantRun, wantWorkspace := pgconv.UUIDString(run.ID), pgconv.UUIDString(run.WorkspaceID)

	if got := executeArgs(run); got.RunID != wantRun || got.WorkspaceID != wantWorkspace {
		t.Errorf("driving queues run %s in workspace %s, want %s in %s",
			got.RunID, got.WorkspaceID, wantRun, wantWorkspace)
	}
	if got := cleanupArgs(run); got.RunID != wantRun || got.WorkspaceID != wantWorkspace {
		t.Errorf("cleaning queues run %s in workspace %s, want %s in %s",
			got.RunID, got.WorkspaceID, wantRun, wantWorkspace)
	}
}
