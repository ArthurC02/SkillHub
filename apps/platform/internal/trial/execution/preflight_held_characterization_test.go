package run

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/design"
)

func TestAHeldTestCaseOfAnotherSkillIsNotFound(t *testing.T) {
	var ws, skill, otherSkill, versionID, testCaseID pgtype.UUID
	for i, u := range []*pgtype.UUID{&ws, &skill, &otherSkill, &versionID, &testCaseID} {
		u.Bytes[0], u.Valid = byte(i+1), true
	}
	held := &heldInputs{
		version: VersionFacts{ID: versionID, SkillID: skill},
		draft:   testlab.Draft{TestCaseID: testCaseID, SkillID: otherSkill},
	}

	_, err := (&Service{}).permissionSummaryFor(context.Background(), ws, preflightTarget{
		skillID: skill, versionID: versionID, testCaseID: testCaseID,
	}, held)

	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}
