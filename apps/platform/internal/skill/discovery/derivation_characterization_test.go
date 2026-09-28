package catalog

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestADetailSaysWhetherTheSkillIsAForkAndOfWhat(t *testing.T) {
	skillID := pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true}
	fork, original := ForkDerivation(), OriginalDerivation()
	for _, tc := range []struct {
		name  string
		skill SkillFacts
		want  derivationInfo
	}{
		{"an original skill", SkillFacts{}, derivationInfo{Label: original.Label, Note: original.Note}},
		{"a fork names its origin", SkillFacts{ForkedFromSkillID: skillID, ForkedFromVersionID: versionID}, derivationInfo{
			IsFork: true, Label: fork.Label, Note: fork.Note,
			ForkedFromSkillID:   "aa000000-0000-0000-0000-000000000000",
			ForkedFromVersionID: "bb000000-0000-0000-0000-000000000000",
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := derivation(tc.skill); got != tc.want {
				t.Fatalf("derivation = %+v, want %+v", got, tc.want)
			}
		})
	}
}
