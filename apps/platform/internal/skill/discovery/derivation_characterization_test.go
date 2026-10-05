package catalog

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestADetailSaysWhetherTheSkillIsAForkAndOfWhat(t *testing.T) {
	skillID := pgtype.UUID{Bytes: [16]byte{0xaa}, Valid: true}
	versionID := pgtype.UUID{Bytes: [16]byte{0xbb}, Valid: true}
	for _, tc := range []struct {
		name  string
		skill SkillFacts
		want  derivationInfo
	}{
		{"an original skill", SkillFacts{}, derivationInfo{
			Label: "原始 Skill", Note: "非任何既有 Skill 的分岔。",
		}},
		{"a fork names its origin", SkillFacts{ForkedFromSkillID: skillID, ForkedFromVersionID: versionID}, derivationInfo{
			IsFork: true, Label: "衍生自其他 Skill",
			Note:                "顯示原始 Skill 與分岔當下的版本;原始版本之後的變更不會自動同步。",
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
