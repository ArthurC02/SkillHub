package apiserver

import (
	"context"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func TestTheCreationDraftCheckReachesTheSessionHashReportAndVerdictInPlace(t *testing.T) {
	valid := creation.GeneratedSkill{
		Name:        "draft-check-summary",
		Description: "Summarize the input in the requested format. Use when the user asks for a summary.",
		Body:        "# Summary\n\n1. Read the input.\n2. Summarize the important points.\n",
	}
	withSecondManifest := valid
	withSecondManifest.Files = []creation.GeneratedFile{{Path: "SKILL.md", Content: "---\nlicense: MIT\n---\n"}}

	for name, c := range map[string]struct {
		draft       creation.GeneratedSkill
		wantBlocked bool
	}{
		"a draft that packages":           {valid, false},
		"a draft that cannot be packaged": {withSecondManifest, true},
	} {
		t.Run(name, func(t *testing.T) {
			versions := &ingest.Service{}
			s := &creation.Service{}
			wireCreationReads(s, versions, nil)

			want, wantErr := versions.ValidateCreationDraft(context.Background(), wiring.GeneratedSkillForIngest(c.draft))
			hash, report, blocked, err := s.ValidateDraft(context.Background(), c.draft)
			if err != nil || wantErr != nil {
				t.Fatalf("err = %v, admission err = %v; want neither", err, wantErr)
			}
			if hash != want.ContentHash || report != want.Report || blocked != want.Blocked {
				t.Fatalf("session saw hash=%q report=%q blocked=%v, admission said %+v", hash, report, blocked, want)
			}
			if blocked != c.wantBlocked || report == "" || (hash == "") != c.wantBlocked {
				t.Fatalf("hash=%q blocked=%v report=%q, want blocked=%v with a report and a hash only when it packages", hash, blocked, report, c.wantBlocked)
			}
			if c.wantBlocked && !strings.Contains(report, "SKILL.md") {
				t.Fatalf("the refusal does not say what to change: %s", report)
			}
		})
	}
}
