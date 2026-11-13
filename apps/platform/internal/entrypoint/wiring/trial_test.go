package wiring

import (
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission/mcptools"
	registry "github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/readmodel"
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
			got := evalRunFacts(readmodel.EvaluationRun{Status: tc.status, Terminal: tc.finished, Succeeded: tc.succeeded})
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
		Compatibility: "Linux\nskillhub-web-search",
	}
	want := run.VersionFacts{ID: version.ID, SkillID: version.SkillID, ContentHash: "sha256:abc",
		PackageObjectKey: "packages/abc.zip", SourcePath: "skills/report", Compatibility: "Linux\nskillhub-web-search"}
	if got := runVersionFacts(version); !reflect.DeepEqual(got, want) {
		t.Errorf("run facts for a version = %+v, want %+v", got, want)
	}
}

func TestRunStandInsCarryEachListedToolUnderItsReference(t *testing.T) {
	got := runStandIns([]mcptools.Tool{
		{Server: "calendar", Tool: "create_event", Purpose: "Creates an event.", Parameters: "title, start", ServerURL: "https://mcp.example.com"},
		{Server: "mail", Tool: "send", Purpose: "Sends mail."},
	})
	want := []run.MCPToolStandIn{
		{Reference: "mcp__calendar__create_event", Purpose: "Creates an event.", Parameters: "title, start"},
		{Reference: "mcp__mail__send", Purpose: "Sends mail."},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("runStandIns = %+v, want %+v", got, want)
	}
	if got := runStandIns(nil); got != nil {
		t.Errorf("runStandIns(nil) = %+v, want nil so the request omits mcp_tools", got)
	}
}
