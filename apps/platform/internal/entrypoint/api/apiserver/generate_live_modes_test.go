package apiserver_test

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func TestARealGatewayGeneratesFromADiagramAndFromAReference(t *testing.T) {
	base, diagramPath, outDir := os.Getenv("SKILLHUB_E2E_LLM_URL"), os.Getenv("SKILLHUB_E2E_DIAGRAM"), os.Getenv("SKILLHUB_E2E_OUT")
	if base == "" || diagramPath == "" || outDir == "" {
		t.Skip("set SKILLHUB_E2E_LLM_URL, SKILLHUB_E2E_DIAGRAM and SKILLHUB_E2E_OUT; this test spends money")
	}
	png, err := os.ReadFile(diagramPath)
	if err != nil {
		t.Fatal(err)
	}
	pool := requireDB(t)
	a := newAPIWithLLM(t, pool, base)
	c := a.login(t, "gen-real-modes")
	ws := workspaceOf(t, pool, c)
	ctx := context.Background()

	inspect := func(label string, res ingest.GenerateResult) {
		t.Helper()
		if res.Report.Blocked {
			t.Logf("%s: blocked after %d attempt(s): %+v", label, res.Attempts, res.Report.Findings)
			return
		}
		data, err := a.packages.Get(ctx, res.Version.PackageObjectKey)
		if err != nil {
			t.Fatalf("%s: stored package unreadable: %v", label, err)
		}
		fsys, err := skillpkg.PackageFS(data)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		md, err := fs.ReadFile(fsys, "SKILL.md")
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		if err := os.WriteFile(filepath.Join(outDir, label+".SKILL.md"), md, 0o600); err != nil {
			t.Fatal(err)
		}
		var inputs []byte
		if err := pool.QueryRow(ctx, `
			SELECT s.generation_inputs FROM skill_sources s JOIN skill_versions v ON v.source_id = s.id
			WHERE v.id = $1`, res.Version.ID).Scan(&inputs); err != nil {
			t.Fatal(err)
		}
		if len(inputs) == 0 {
			t.Errorf("%s: generation_inputs is NULL; ADR-066 says every non-text input leaves a record", label)
		}
		cost := "unpriced"
		if res.CostUSD != nil {
			cost = fmt.Sprintf("US$%.6f", *res.CostUSD)
		}
		t.Logf("%s: version %d, attempts=%d, model=%s, prompt=%s, %s, generation_inputs=%s",
			label, res.Version.VersionNumber, res.Attempts, res.Model, res.PromptVersion, cost, inputs)
	}

	res, err := a.versions.GenerateSkill(ctx, ws, ingest.GenerateInput{
		Diagram: &ingest.GenerateDiagram{MediaType: "image/png", Data: png},
	})
	if err != nil {
		t.Fatalf("diagram-only generation against a real gateway: %v", err)
	}
	inspect("diagram-only", res)

	refSkillID, _ := importFiles(t, a, pool, c, map[string]string{
		"SKILL.md": "---\nname: weekly-status-digest\ndescription: Turn a week of chat messages into a status digest. Use when asked for a weekly summary.\n---\n\n" +
			"## 步驟\n\n1. 先列出本週所有訊息的日期與作者。\n2. 依「已完成／進行中／卡住」三欄分組。\n3. 每欄最多五條，一條一句，句尾附訊息日期。\n4. 最後一段列出下週要追的事。\n\n## 輸出格式\n\nMarkdown，三個二級標題，各一個清單。\n",
	})
	res, err = a.versions.GenerateSkill(ctx, ws, ingest.GenerateInput{
		TaskDescription:   "把客戶寄來的會議錄音逐字稿整理成待辦清單，每一條要有負責人與期限。",
		ReferenceSkillIDs: []pgtype.UUID{mustUUID(t, refSkillID)},
	})
	if err != nil {
		t.Fatalf("reference generation against a real gateway: %v", err)
	}
	inspect("with-reference", res)
}
