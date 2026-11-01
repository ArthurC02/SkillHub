package ingest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
)

func TestACreationDraftWhoseReferenceIsOnlyReachableThroughAnotherIsRefusedWithTheFileNamed(t *testing.T) {
	g := goodGeneratedSkill()
	g.Body += "3. 依 [references/rules.md](references/rules.md) 判斷級距。\n"
	g.Files = []GeneratedFile{
		{Path: "references/rules.md", Content: "詳見 [details](details.md)。\n"},
		{Path: "references/details.md", Content: "級距表\n"},
	}
	check, err := (&Service{}).ValidateCreationDraft(context.Background(), g)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !check.Blocked || check.ContentHash == "" {
		t.Fatalf("blocked=%v hash=%q, want a refused draft that still packaged", check.Blocked, check.ContentHash)
	}
	var report skillpkg.Report
	if err := json.Unmarshal([]byte(check.Report), &report); err != nil {
		t.Fatalf("report is not JSON: %v", err)
	}
	found := false
	for _, f := range report.Findings {
		if f.Code == skillpkg.CodeNestedReference {
			found = f.Path == "references/details.md" && f.Severity == skillpkg.SeverityError
		}
	}
	if !found || !strings.Contains(check.Report, "references/rules.md") {
		t.Fatalf("the refusal does not name the nested file as an error: %s", check.Report)
	}
}

func TestACreationDraftKeepsOtherWarningsAsWarnings(t *testing.T) {
	g := goodGeneratedSkill()
	g.Body += "3. 讀 references/missing.md。\n"
	g.Files = []GeneratedFile{{Path: "references/present.md", Content: "x\n"}}
	check, err := (&Service{}).ValidateCreationDraft(context.Background(), g)
	if err != nil || check.Blocked {
		t.Fatalf("blocked=%v err=%v, want a draft whose only finding is a missing-file warning to pass: %s", check.Blocked, err, check.Report)
	}
	if !strings.Contains(check.Report, `"severity":"warning","code":"file-ref-missing"`) {
		t.Fatalf("the missing-file warning was not kept as a warning: %s", check.Report)
	}
}
