package apiserver_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
)

func TestAVersionNamingADifferentSkillIsRefusedAndAddsNothing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "version-name-mismatch")
	ws := workspaceOf(t, pool, c)
	ctx := context.Background()
	first, err := a.versions.UploadZip(ctx, ws, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: tidy-notes\ndescription: The first summary.\n---\n\nDo it.\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	skill := onlyImported(t, first).Skill

	res, err := a.versions.SaveVersion(ctx, ws, skill.ID, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: split-csv\ndescription: Another skill's summary.\n---\n\nSplit it.\n",
	}))
	if err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	if !res.Report.Blocked || !slices.Contains(findingCodes(res.Report.Findings), skillpkg.CodeVersionNameMismatch) {
		t.Fatalf("report = %+v, want blocked with %s", res.Report, skillpkg.CodeVersionNameMismatch)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM skill_versions WHERE skill_id = $1`, skill.ID); n != 1 {
		t.Errorf("the skill has %d versions, want its one upload", n)
	}
	var summary string
	if err := pool.QueryRow(ctx, `SELECT summary FROM skills WHERE id = $1`, skill.ID).Scan(&summary); err != nil {
		t.Fatal(err)
	}
	if summary != "The first summary." {
		t.Errorf("summary = %q, want the skill's own; the other skill's description took it over", summary)
	}
}

func TestATakenDownSkillTakesNoNewVersion(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "version-taken-down")
	ws := workspaceOf(t, pool, c)
	ctx := context.Background()
	first, err := a.versions.UploadZip(ctx, ws, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: tidy-notes\ndescription: The first summary.\n---\n\nDo it.\n",
	}))
	if err != nil {
		t.Fatal(err)
	}
	skill := onlyImported(t, first).Skill
	if _, err := pool.Exec(ctx, `UPDATE skills SET takedown_at = now(), takedown_reason = 'test' WHERE id = $1`, skill.ID); err != nil {
		t.Fatal(err)
	}

	res, err := a.versions.SaveVersion(ctx, ws, skill.ID, zipOf(t, map[string]string{
		"SKILL.md": "---\nname: tidy-notes\ndescription: The second summary.\n---\n\nDo it better.\n",
	}))
	if err != nil {
		t.Fatalf("SaveVersion: %v", err)
	}
	if !res.Report.Blocked || !slices.Contains(findingCodes(res.Report.Findings), skillpkg.CodeSkillTakenDown) {
		t.Fatalf("report = %+v, want blocked with %s", res.Report, skillpkg.CodeSkillTakenDown)
	}
	if n := countRow(t, pool, `SELECT count(*) FROM skill_versions WHERE skill_id = $1`, skill.ID); n != 1 {
		t.Errorf("the taken-down skill has %d versions, want its one upload", n)
	}
}

func TestAReimportRefusesOnlyTheTakenDownSkillOfItsSource(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "reimport-taken-down")
	source := func(edit string) []byte {
		return zipOf(t, map[string]string{
			"plugin.json":                conformingPlugin("desk-tools"),
			"skills/tidy-notes/SKILL.md": skillNamed("tidy-notes") + edit,
			"skills/split-csv/SKILL.md":  skillNamed("split-csv") + edit,
		})
	}
	if code, body := postSource(t, owner, source("")); code != http.StatusCreated {
		t.Fatalf("first import: %d %v", code, body)
	}
	ws := workspaceOf(t, pool, owner)
	if _, err := pool.Exec(context.Background(),
		`UPDATE skills SET takedown_at = now(), takedown_reason = 'test' WHERE workspace_id = $1 AND name = 'tidy-notes'`, ws.ID); err != nil {
		t.Fatal(err)
	}

	if code, body := postSource(t, owner, source("\nEdited upstream.\n")); code != http.StatusCreated {
		t.Fatalf("re-import: %d %v, want the untouched skill still imported", code, body)
	}
	versions := func(name string) int {
		return countRow(t, pool, `SELECT count(*) FROM skill_versions v JOIN skills s ON s.id = v.skill_id
			WHERE s.workspace_id = $1 AND s.name = $2`, ws.ID, name)
	}
	if n := versions("tidy-notes"); n != 1 {
		t.Errorf("the taken-down skill has %d versions after a re-import, want 1", n)
	}
	if n := versions("split-csv"); n != 2 {
		t.Errorf("the other skill of the source has %d versions, want 2; one refusal stopped the whole import", n)
	}
}

func TestARevisedGeneratedCandidateKeepsTheSkillsName(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "candidate-keeps-name")
	ws := workspaceOf(t, pool, c)
	ctx := context.Background()
	draft := ingest.GeneratedSkill{
		Name:        "scan-to-table",
		Description: "把掃描的單據影像抽成表格。當使用者手上是掃描件、需要彙整成一份時使用。",
		Body:        "# 內容\n\n1. 做這件事。\n",
	}
	provenance := ingest.GeneratedCandidateProvenance{TaskDescription: "抽出表格。", Model: "fixture-model", PromptVersion: "fixture-prompt"}
	first, err := a.versions.MaterializeGeneratedCandidate(ctx, ws, draft, provenance, nil)
	if err != nil || first.Report.Blocked {
		t.Fatalf("first candidate: %+v, %v", first.Report, err)
	}
	target := first.Skill.ID

	draft.Name, draft.Body = "scan-into-table", "# 內容\n\n1. 做得更好。\n"
	provenance.ExistingSkillID = &target
	revised, err := a.versions.MaterializeGeneratedCandidate(ctx, ws, draft, provenance, nil)
	if err != nil || revised.Report.Blocked {
		t.Fatalf("revised candidate: %+v, %v; a renamed draft must still save onto its skill", revised.Report, err)
	}
	var name string
	if err := pool.QueryRow(ctx, `SELECT manifest->>'name' FROM skill_versions WHERE skill_id = $1 ORDER BY version_number DESC LIMIT 1`,
		target).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "scan-to-table" {
		t.Errorf("newest version names %q, want the skill's own name scan-to-table", name)
	}
}
