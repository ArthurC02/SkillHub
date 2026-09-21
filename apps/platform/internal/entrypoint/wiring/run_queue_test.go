package wiring

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

func TestAnAbsentQueueDoesNotLookConfigured(t *testing.T) {
	var client *river.Client[pgx.Tx]
	if queue := NewRunQueue(client); queue != nil {
		t.Error("an unconfigured client became a non-nil RunQueue")
	}
}

func TestQueuedWorkNamesTheRunAndWorkspace(t *testing.T) {
	work := run.RunWork{RunID: pgtype.UUID{Bytes: [16]byte{0: 1}, Valid: true}, WorkspaceID: pgtype.UUID{Bytes: [16]byte{0: 2}, Valid: true}}
	if got := runJob(work); got.RunID != pgconv.UUIDString(work.RunID) || got.WorkspaceID != pgconv.UUIDString(work.WorkspaceID) {
		t.Errorf("run job = %+v", got)
	}
	if got := cleanupJob(work); got.RunID != pgconv.UUIDString(work.RunID) || got.WorkspaceID != pgconv.UUIDString(work.WorkspaceID) {
		t.Errorf("cleanup job = %+v", got)
	}
}
