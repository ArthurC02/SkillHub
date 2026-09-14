package wiring

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func TestEvaluationTreatsARunAsFinishedExactlyInItsFourEndStates(t *testing.T) {
	for _, tc := range []struct {
		status   gen.RunStatus
		finished bool
	}{
		{gen.RunStatusQueued, false},
		{gen.RunStatusProvisioning, false},
		{gen.RunStatusPreparing, false},
		{gen.RunStatusRunning, false},
		{gen.RunStatusEvaluating, false},
		{gen.RunStatusSucceeded, true},
		{gen.RunStatusFailed, true},
		{gen.RunStatusCancelled, true},
		{gen.RunStatusTimedOut, true},
	} {
		t.Run(string(tc.status), func(t *testing.T) {
			if got := evalRunFacts(run.EvaluationRun{Status: string(tc.status)}).Terminal; got != tc.finished {
				t.Errorf("evaluation sees %s as finished = %v, want %v", tc.status, got, tc.finished)
			}
		})
	}
}

func TestTrialContextsSeeTheOwnersRestrictionVerdict(t *testing.T) {
	hold, summary := "license-review", "summary"
	id := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	held := registry.Skill{ID: id, Name: "name", Summary: &summary, AccessRestriction: &hold}

	if got, want := runSkillFacts(held), (run.SkillFacts{AccessRestricted: true, AccessRestrictionReason: hold}); got != want {
		t.Errorf("run facts for a held skill = %+v, want %+v", got, want)
	}
	if got, want := evalSkillFacts(held), (eval.SkillFacts{ID: id, Name: "name", Summary: &summary, AccessRestricted: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("evaluation facts for a held skill = %+v, want %+v", got, want)
	}
	if got := runSkillFacts(registry.Skill{}); got != (run.SkillFacts{}) {
		t.Errorf("run facts for a skill under no hold = %+v", got)
	}
	if got := evalSkillFacts(registry.Skill{}); got.AccessRestricted {
		t.Errorf("evaluation facts for a skill under no hold = %+v", got)
	}
}
