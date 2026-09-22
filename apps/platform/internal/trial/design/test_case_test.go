package testlab

import (
	"bytes"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestTestCaseOfPreservesThePersistentTestCase(t *testing.T) {
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	skillID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	createdAt := pgtype.Timestamptz{Time: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), Valid: true}
	updatedAt := pgtype.Timestamptz{Time: time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC), Valid: true}
	deletedAt := pgtype.Timestamptz{Time: time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC), Valid: true}
	row := gen.TestCase{
		ID: id, WorkspaceID: workspaceID, SkillID: skillID, Name: "name", UserPrompt: "prompt",
		AcceptanceCriteria: []byte(`[{"id":"criterion"}]`), Rubric: []byte(`{"version":"v1"}`),
		CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: deletedAt,
	}

	got := testCaseOf(row)
	if got.ID != id || got.WorkspaceID != workspaceID || got.SkillID != skillID ||
		got.Name != "name" || got.UserPrompt != "prompt" || got.CreatedAt != createdAt ||
		got.UpdatedAt != updatedAt || got.DeletedAt != deletedAt ||
		!bytes.Equal(got.AcceptanceCriteria, row.AcceptanceCriteria) || !bytes.Equal(got.Rubric, row.Rubric) {
		t.Fatalf("testCaseOf() = %+v, want all TestCase fields preserved", got)
	}
}
