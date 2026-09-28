package publishing

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestANonListableSnapshotIsNotExposed(t *testing.T) {
	versionID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	workspaceID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	skillID := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	state := ExposureState{
		Status: StatusPublished, SkillID: skillID, OwnerWorkspaceID: workspaceID,
		VersionID: versionID, Approved: true, Concluded: true, ReviewedDigest: "reviewed",
	}
	svc := Service{
		ReadSkill: func(context.Context, pgtype.UUID, pgtype.UUID) (SkillFacts, bool, error) {
			return SkillFacts{ID: skillID, Redistribution: redistributionAllowed}, true, nil
		},
		ReadSearchSnapshot: func(context.Context, pgtype.UUID) (SearchSnapshot, bool, error) {
			return SearchSnapshot{VersionID: versionID, Digest: "reviewed", Listable: false}, true, nil
		},
	}

	exposed, _, err := svc.exposedNow(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if exposed {
		t.Fatal("non-listable search text is exposed")
	}
	projection, err := svc.catalogExposure(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	if projection.State != CatalogSearchNotReady {
		t.Fatalf("owner state = %q, want %q", projection.State, CatalogSearchNotReady)
	}
}
