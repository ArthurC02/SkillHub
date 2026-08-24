package identity

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
)

func TestPublishedDTOsPreserveIdentityFacts(t *testing.T) {
	userID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	ownerID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	createdAt := pgtype.Timestamptz{Valid: true}
	updatedAt := pgtype.Timestamptz{Valid: true}
	deletedAt := pgtype.Timestamptz{Valid: true}
	requestedAt := pgtype.Timestamptz{Valid: true}

	user := userDTO(gen.User{
		ID: userID, Email: "maker@example.com", DisplayName: "Maker",
		CreatedAt: createdAt, UpdatedAt: updatedAt, DeletedAt: deletedAt,
		DeletionRequestedAt: requestedAt,
	})
	if user.ID != userID || user.Email != "maker@example.com" || user.DisplayName != "Maker" ||
		user.CreatedAt != createdAt || user.UpdatedAt != updatedAt || user.DeletedAt != deletedAt ||
		user.DeletionRequestedAt != requestedAt {
		t.Fatalf("userDTO() = %#v", user)
	}

	workspace := workspaceDTO(gen.Workspace{
		ID: workspaceID, OwnerUserID: ownerID, Name: "Personal",
		CreatedAt: createdAt, UpdatedAt: updatedAt, IsCatalog: true,
	})
	if workspace.ID != workspaceID || workspace.OwnerUserID != ownerID || workspace.Name != "Personal" ||
		workspace.CreatedAt != createdAt || workspace.UpdatedAt != updatedAt || !workspace.IsCatalog {
		t.Fatalf("workspaceDTO() = %#v", workspace)
	}
}
