package ingest

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/observability/audit"
	"github.com/ArthurC02/skillhub/apps/platform/internal/skill/library"
)

func TestAVersionAuditNamesTheActionTheVersionAndWhatTheCallerAdded(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var ws identity.Workspace
	if err := tx.QueryRow(ctx,
		`INSERT INTO users (email, display_name) VALUES ('version-audit@example.test', 'va') RETURNING id`,
	).Scan(&ws.OwnerUserID); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO workspaces (owner_user_id, name) VALUES ($1, 'version-audit') RETURNING id`, ws.OwnerUserID,
	).Scan(&ws.ID); err != nil {
		t.Fatal(err)
	}
	skillID := mustUUIDForTest(t, "20000000-0000-0000-0000-00000000a0d2")
	versionID := mustUUIDForTest(t, "30000000-0000-0000-0000-00000000a0d2")
	res := Result{
		Skill:     registry.Skill{ID: skillID},
		Version:   registry.Version{ID: versionID, ContentHash: "abc123"},
		Duplicate: true,
	}

	err = auditVersion(ctx, tx, ws, res, versionAudit{audit.ActionSkillVersionCreate, map[string]any{"version_number": 3}})
	if err != nil {
		t.Fatal(err)
	}

	var actor, workspaceID, resourceID pgtype.UUID
	var action, resourceType, skill, duplicate, hash, number string
	if err := tx.QueryRow(ctx,
		`SELECT actor_user_id, workspace_id, action, resource_type, resource_id,
		        metadata->>'skill_id', metadata->>'duplicate', metadata->>'content_hash', metadata->>'version_number'
		   FROM audit_events WHERE workspace_id = $1`, ws.ID,
	).Scan(&actor, &workspaceID, &action, &resourceType, &resourceID, &skill, &duplicate, &hash, &number); err != nil {
		t.Fatalf("no single audit row: %v", err)
	}
	if actor != ws.OwnerUserID || workspaceID != ws.ID || action != audit.ActionSkillVersionCreate ||
		resourceType != audit.ResourceVersion || resourceID != versionID {
		t.Errorf("actor=%v workspace=%v action=%q resource=%q/%v, want the owner, the workspace, %q and %q/%v",
			actor, workspaceID, action, resourceType, resourceID, audit.ActionSkillVersionCreate, audit.ResourceVersion, versionID)
	}
	if skill != "20000000-0000-0000-0000-00000000a0d2" || duplicate != "true" || hash != "abc123" || number != "3" {
		t.Errorf("metadata skill_id=%q duplicate=%q content_hash=%q version_number=%q", skill, duplicate, hash, number)
	}
}

func removeAdmissionWorkspace(t *testing.T, pool *pgxpool.Pool, ws identity.Workspace) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Errorf("cleanup: %v", err)
			return
		}
		defer func() { _ = tx.Rollback(ctx) }()
		for _, step := range []struct {
			stmt string
			args []any
		}{
			{`SET LOCAL skillhub.purge = 'on'`, nil},
			{`DELETE FROM audit_events WHERE workspace_id = $1 OR actor_user_id = $2`, []any{ws.ID, ws.OwnerUserID}},
			{`DELETE FROM skill_versions WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM skills WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM skill_sources WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM outbox_events WHERE workspace_id = $1`, []any{ws.ID}},
			{`DELETE FROM workspaces WHERE id = $1`, []any{ws.ID}},
			{`DELETE FROM users WHERE id = $1`, []any{ws.OwnerUserID}},
		} {
			if _, err := tx.Exec(ctx, step.stmt, step.args...); err != nil {
				t.Errorf("cleanup %q: %v", step.stmt, err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Errorf("cleanup: %v", err)
		}
	})
}

func versionAuditAction(t *testing.T, pool *pgxpool.Pool, versionID pgtype.UUID) (action, sourceType string) {
	t.Helper()
	var st *string
	if err := pool.QueryRow(context.Background(),
		`SELECT action, metadata->>'source_type' FROM audit_events WHERE resource_id = $1`, versionID,
	).Scan(&action, &st); err != nil {
		t.Fatalf("no single audit row for version %v: %v", versionID, err)
	}
	if st != nil {
		sourceType = *st
	}
	return action, sourceType
}

func auditTestService(pool *pgxpool.Pool) *Service {
	return &Service{
		Pool:       pool,
		Store:      &creationTestStore{},
		IndexSkill: func(context.Context, pgx.Tx, SkillProjection) error { return nil },
	}
}

func TestEveryImportPathIndexesTheSummaryAndScanItPrepared(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "projection-carries-enrichment")
	removeAdmissionWorkspace(t, pool, ws)
	var indexed []SkillProjection
	svc := auditTestService(pool)
	svc.IndexSkill = func(_ context.Context, _ pgx.Tx, p SkillProjection) error {
		indexed = append(indexed, p)
		return nil
	}
	uploaded, err := svc.UploadZip(ctx, ws, zipBytes(t, map[string]string{"SKILL.md": skillMD}))
	if err != nil || len(uploaded.Imported) != 1 {
		t.Fatalf("upload: imported=%d err=%v", len(uploaded.Imported), err)
	}
	if _, err := svc.SaveVersion(ctx, ws, uploaded.Imported[0].Skill.ID, zipBytes(t, map[string]string{"SKILL.md": skillMD + "\nMore.\n"})); err != nil {
		t.Fatal(err)
	}
	prov := GeneratedCandidateProvenance{TaskDescription: "test task", Model: "test-model", PromptVersion: "v1"}
	generated, err := svc.MaterializeGeneratedCandidate(ctx, ws, goodGeneratedSkill(), prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	revised := goodGeneratedSkill()
	revised.Body += "3. 合併成一份檔案。\n"
	prov.ExistingSkillID = &generated.Skill.ID
	if _, err := svc.MaterializeGeneratedCandidate(ctx, ws, revised, prov, nil); err != nil {
		t.Fatal(err)
	}

	want := []string{"Work with PDFs.", "Work with PDFs.", goodGeneratedSkill().Description, goodGeneratedSkill().Description}
	if len(indexed) != len(want) {
		t.Fatalf("indexed %d projections, want %d (upload, saved version, generated, revision)", len(indexed), len(want))
	}
	for i, p := range indexed {
		if p.Summary != want[i] || string(p.Scan) == "" || p.EnrichmentStatus != string(enrichmentPending) {
			t.Errorf("projection %d summary=%q scan=%q status=%q, want %q, the scan facts and %q",
				i, p.Summary, p.Scan, p.EnrichmentStatus, want[i], enrichmentPending)
		}
	}
}

func TestANewVersionSavedOverAnUploadIsAuditedAsAVersionCreate(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "version-audit-save")
	removeAdmissionWorkspace(t, pool, ws)
	svc := auditTestService(pool)
	uploaded, err := svc.UploadZip(ctx, ws, zipBytes(t, map[string]string{"SKILL.md": skillMD}))
	if err != nil || len(uploaded.Imported) != 1 {
		t.Fatalf("upload: imported=%d err=%v", len(uploaded.Imported), err)
	}

	saved, err := svc.SaveVersion(ctx, ws, uploaded.Imported[0].Skill.ID, zipBytes(t, map[string]string{"SKILL.md": skillMD + "\nMore.\n"}))
	if err != nil {
		t.Fatal(err)
	}

	if action, _ := versionAuditAction(t, pool, saved.Version.ID); action != audit.ActionSkillVersionCreate {
		t.Errorf("saved version audited as %q, want %q", action, audit.ActionSkillVersionCreate)
	}
}

func TestARevisedGeneratedSkillIsAuditedAsAGeneratedImport(t *testing.T) {
	pool := requireCreationDB(t)
	ctx := context.Background()
	ws := seedCreationWorkspace(t, pool, "version-audit-revision")
	removeAdmissionWorkspace(t, pool, ws)
	svc := auditTestService(pool)
	prov := GeneratedCandidateProvenance{TaskDescription: "test task", Model: "test-model", PromptVersion: "v1"}
	first, err := svc.MaterializeGeneratedCandidate(ctx, ws, goodGeneratedSkill(), prov, nil)
	if err != nil {
		t.Fatal(err)
	}
	revised := goodGeneratedSkill()
	revised.Body += "3. 合併成一份檔案。\n"
	prov.ExistingSkillID = &first.Skill.ID

	second, err := svc.MaterializeGeneratedCandidate(ctx, ws, revised, prov, nil)
	if err != nil || second.Duplicate {
		t.Fatalf("revision: duplicate=%v err=%v", second.Duplicate, err)
	}

	action, sourceType := versionAuditAction(t, pool, second.Version.ID)
	if action != audit.ActionSkillImport || sourceType != string(SourceGenerated) {
		t.Errorf("revision audited as %q source_type=%q, want %q %q", action, sourceType, audit.ActionSkillImport, SourceGenerated)
	}
}
