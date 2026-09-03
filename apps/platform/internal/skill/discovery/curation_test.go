package catalog

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func uuid(t *testing.T, s string) pgtype.UUID {
	t.Helper()
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		t.Fatal(err)
	}
	return u
}

// The detail view resolves the 0042 verdict in Go while search resolves the same
// rule in SQL. Two implementations of one rule is the shape this repo keeps
// getting caught by, so this is the Go one's own test: the SQL one is covered by
// TestACuratedSkillSaysSoUntilANewVersionArrives in the apiserver suite.
func TestCurationTierNeedsBothHalvesOfTheRecord(t *testing.T) {
	reviewed := uuid(t, "11111111-1111-4111-8111-111111111111")
	newer := uuid(t, "22222222-2222-4222-8222-222222222222")

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
		// Reachable only through 0042's ON DELETE SET NULL, when the reviewed
		// version was purged. Fail closed: the bytes somebody read can no longer
		// be produced, so the badge cannot be about them.
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
