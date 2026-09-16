package registry

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestPurgesRefuseWithoutTheReferenceReads(t *testing.T) {
	ctx := context.Background()
	if err := (&Service{}).PurgeWorkspace(ctx, nil, pgtype.UUID{}); err == nil {
		t.Error("PurgeWorkspace ran without knowing what still references a skill")
	}
	if _, err := (&Service{}).PurgeDeletedSkills(ctx, time.Hour, 10); err == nil {
		t.Error("PurgeDeletedSkills ran without knowing what still references a skill")
	}
}

func TestAPurgeKeepsASkillWhileAnythingStillHoldsItOrOneOfItsVersions(t *testing.T) {
	skill := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	first := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
	second := pgtype.UUID{Bytes: [16]byte{3}, Valid: true}
	candidate := purgeCandidate{skillID: skill, versionIDs: []pgtype.UUID{first, second}}
	for _, tc := range []struct {
		what     string
		skills   []pgtype.UUID
		versions []pgtype.UUID
		keep     bool
	}{
		{"nothing refers to it", nil, nil, false},
		{"the skill itself is held", []pgtype.UUID{skill}, nil, true},
		{"its newest version is held", nil, []pgtype.UUID{second}, true},
		{"its first version is held", nil, []pgtype.UUID{first}, true},
		{"only another skill's version is held", nil, []pgtype.UUID{{Bytes: [16]byte{9}, Valid: true}}, false},
	} {
		holds := purgeHolds{skills: map[pgtype.UUID]bool{}, versions: map[pgtype.UUID]bool{}}
		for _, id := range tc.skills {
			holds.skills[id] = true
		}
		for _, id := range tc.versions {
			holds.versions[id] = true
		}
		if got := holds.keep(candidate); got != tc.keep {
			t.Errorf("%s: keep = %v, want %v", tc.what, got, tc.keep)
		}
	}
}
