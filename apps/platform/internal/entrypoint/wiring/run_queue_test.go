package wiring

import (
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/riverqueue/river"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
)

func TestAnAbsentQueueDoesNotLookConfigured(t *testing.T) {
	var client *river.Client[pgx.Tx]
	if queue := NewRunQueue(client); queue != nil {
		t.Error("an unconfigured client became a non-nil RunQueue")
	}
}

func TestQueuedWorkNamesTheRunAndWorkspace(t *testing.T) {
	r := gen.Run{ID: pgtype.UUID{Bytes: [16]byte{0: 1}, Valid: true}, WorkspaceID: pgtype.UUID{Bytes: [16]byte{0: 2}, Valid: true}}
	if got := runJob(r); got.RunID != pgconv.UUIDString(r.ID) || got.WorkspaceID != pgconv.UUIDString(r.WorkspaceID) {
		t.Errorf("run job = %+v", got)
	}
	if got := cleanupJob(r); got.RunID != pgconv.UUIDString(r.ID) || got.WorkspaceID != pgconv.UUIDString(r.WorkspaceID) {
		t.Errorf("cleanup job = %+v", got)
	}
}
