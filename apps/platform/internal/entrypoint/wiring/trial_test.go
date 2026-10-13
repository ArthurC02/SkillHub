package wiring

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement/facts"
)

func TestEvaluationReceivesTheOwnersTerminalVerdict(t *testing.T) {
	for _, tc := range []struct {
		status    string
		finished  bool
		succeeded bool
	}{
		{"succeeded", false, false},
		{"running", true, true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			got := evalRunFacts(run.EvaluationRun{Status: tc.status, Terminal: tc.finished, Succeeded: tc.succeeded})
			if got.Terminal != tc.finished {
				t.Errorf("evaluation sees %s as finished = %v, want %v", tc.status, got.Terminal, tc.finished)
			}
			if got.Succeeded != tc.succeeded {
				t.Errorf("evaluation sees %s as succeeded = %v, want the owner's %v", tc.status, got.Succeeded, tc.succeeded)
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
	if got, want := evalSkillFacts(held), (facts.Skill{ID: id, Name: "name", Summary: &summary, AccessRestricted: true}); !reflect.DeepEqual(got, want) {
		t.Errorf("evaluation facts for a held skill = %+v, want %+v", got, want)
	}
	if got := runSkillFacts(registry.Skill{}); got != (run.SkillFacts{}) {
		t.Errorf("run facts for a skill under no hold = %+v", got)
	}
	if got := evalSkillFacts(registry.Skill{}); got.AccessRestricted {
		t.Errorf("evaluation facts for a skill under no hold = %+v", got)
	}
}

func TestRunReceivesTheVersionFactsTheRegistryRecorded(t *testing.T) {
	version := registry.Version{
		ID: pgtype.UUID{Bytes: [16]byte{1}, Valid: true}, SkillID: pgtype.UUID{Bytes: [16]byte{2}, Valid: true},
		ContentHash: "sha256:abc", PackageObjectKey: "packages/abc.zip", SourcePath: "skills/report",
	}
	want := run.VersionFacts{ID: version.ID, SkillID: version.SkillID, ContentHash: "sha256:abc",
		PackageObjectKey: "packages/abc.zip", SourcePath: "skills/report"}
	if got := runVersionFacts(version); got != want {
		t.Errorf("run facts for a version = %+v, want %+v", got, want)
	}
}
