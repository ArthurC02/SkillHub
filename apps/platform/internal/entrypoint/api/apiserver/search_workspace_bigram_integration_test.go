package apiserver_test

import (
	"context"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	catalog "github.com/ArthurC02/skillhub/apps/platform/internal/skill/discovery"
)

func TestWorkspaceSearchFindsAChineseSubstringOfASummary(t *testing.T) {
	pool := requireDB(t)
	owner := newAPI(t, pool).login(t, "owner-workspace-bigram")
	skillID := seedSkill(t, pool, owner.workspaceID, "report-tidier")
	summary := "整理報表資料"
	if err := gen.New(pool).UpsertSearchDocument(context.Background(), gen.UpsertSearchDocumentParams{
		SkillID: mustUUID(t, skillID), WorkspaceID: mustUUID(t, owner.workspaceID),
		Name: "report-tidier", Summary: summary, BigramText: catalog.LexicalIndexText("report-tidier", summary),
	}); err != nil {
		t.Fatal(err)
	}
	svc := &catalog.Service{Pool: pool}

	hits, err := svc.SearchWorkspace(context.Background(), mustUUID(t, owner.workspaceID), "報表", 10)
	if err != nil || len(hits) != 1 || hits[0].SkillID != skillID {
		t.Fatalf("query 報表 = %+v err=%v, want the skill whose summary is %q", hits, err, summary)
	}
	hits, err = svc.SearchWorkspace(context.Background(), mustUUID(t, owner.workspaceID), "財務", 10)
	if err != nil || len(hits) != 0 {
		t.Fatalf("query 財務 = %+v err=%v, want no hits", hits, err)
	}
}
