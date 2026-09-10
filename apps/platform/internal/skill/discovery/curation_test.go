package catalog

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func mustUUID(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

func TestCurationTierNeedsBothHalvesOfTheRecord(t *testing.T) {
	reviewed := mustUUID(t, "11111111-1111-4111-8111-111111111111")
	newer := mustUUID(t, "22222222-2222-4222-8222-222222222222")

	for _, c := range []struct {
		name   string
		skill  SkillFacts
		newest pgtype.UUID
		want   Tier
	}{
		{"reviewed version is still the newest",
			SkillFacts{CurationTier: "curated", CuratedVersionID: reviewed}, reviewed, TierCurated},
		{"the content moved on after the review",
			SkillFacts{CurationTier: "curated", CuratedVersionID: reviewed}, newer, TierIndexed},
		{"no verdict recorded",
			SkillFacts{CurationTier: "indexed"}, reviewed, TierIndexed},

		{"verdict without the version it judged",
			SkillFacts{CurationTier: "curated"}, reviewed, TierIndexed},
		{"a skill with no version at all",
			SkillFacts{CurationTier: "curated", CuratedVersionID: reviewed}, pgtype.UUID{}, TierIndexed},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := curationTier(c.skill, c.newest); got != c.want {
				t.Fatalf("curationTier = %q, want %q", got, c.want)
			}
		})
	}
}
