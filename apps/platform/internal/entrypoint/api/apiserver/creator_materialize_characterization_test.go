package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
)

type materializeProbe struct {
	provenance    creation.Provenance
	testPrompt    string
	beforeCommit  func(ctx context.Context)
	createdSkill  string
	duplicateCost float64
}

func (p *materializeProbe) service(pool *pgxpool.Pool) *creation.Service {
	return &creation.Service{
		Pool:   pool,
		Limits: creationLimits(),
		Materialize: func(ctx context.Context, _ identity.Workspace, _ creation.GeneratedSkill, prov creation.Provenance, commit func(context.Context, pgx.Tx, creation.Candidate) error) error {
			p.provenance = prov
			if p.beforeCommit != nil {
				p.beforeCommit(ctx)
			}
			tx, err := pool.Begin(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = tx.Rollback(ctx) }()
			if err := commit(ctx, tx, creation.Candidate{SkillID: p.createdSkill}); err != nil {
				return err
			}
			return tx.Commit(ctx)
		},
		CreateAcceptanceTestCase: func(_ context.Context, _ pgx.Tx, _ identity.Workspace, _, _, prompt string, _ []string) (string, error) {
			p.testPrompt = prompt
			return "characterized-test-case", nil
		},
		DuplicateCheck: func(context.Context, identity.Workspace, string) ([]creation.Reference, float64, error) {
			return nil, p.duplicateCost, nil
		},
	}
}

func seedSaveReadySession(t *testing.T, pool *pgxpool.Pool, svc *creation.Service, ws identity.Workspace, snapshot map[string]any) pgtype.UUID {
	t.Helper()
	ctx := context.Background()
	id := creationID(t)
	if _, err := svc.Create(ctx, ws, id, "", .5); err != nil {
		t.Fatal(err)
	}
	patch, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE creation_sessions
		SET snapshot = jsonb_set(snapshot, '{snapshot}', (snapshot->'snapshot') || $2::jsonb) || '{"existing_skill_id":"earlier-skill"}'::jsonb
		WHERE id = $1`, id, patch); err != nil {
		t.Fatal(err)
	}
	return id
}

func saveReadySnapshot(sampleInput string) map[string]any {
	return map[string]any{
		"brief":               "把會議紀錄整理成摘要",
		"brief_confirmed":     true,
		"sample_input":        sampleInput,
		"acceptance_criteria": []string{"摘要列出每個決議"},
		"spent_usd":           0.25,
		"draft":               map[string]any{"content_hash": "draft-hash", "skill": map[string]any{"name": "meeting-summary", "description": "summarise"}, "blocked": false},
	}
}

func materializeCommand(t *testing.T) creation.Command {
	return creation.Command{ID: creationID(t), ExpectedRevision: 1, Kind: "materialize", ContentHash: "draft-hash"}
}

func TestAMaterializedCandidateCarriesTheSpendTheSaveCheckAddedAndTheNewSkill(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	probe := &materializeProbe{createdSkill: "characterized-skill", duplicateCost: 0.05}
	svc := probe.service(pool)
	id := seedSaveReadySession(t, pool, svc, ws, saveReadySnapshot("   "))
	command := materializeCommand(t)

	v, _, err := svc.Act(context.Background(), ws, id, command)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _, err := svc.Act(context.Background(), ws, id, command)
	if err != nil || replayed.Revision != v.Revision || replayed.State != v.State {
		t.Errorf("resending the save = revision %d state %s err %v, want the recorded revision %d state %s", replayed.Revision, replayed.State, err, v.Revision, v.State)
	}
	if probe.testPrompt != "把會議紀錄整理成摘要" {
		t.Errorf("acceptance test prompt = %q, want the brief when the sample input is blank", probe.testPrompt)
	}
	if probe.provenance.ExistingSkillID != "earlier-skill" {
		t.Errorf("provenance existing skill = %q, want earlier-skill", probe.provenance.ExistingSkillID)
	}
	if v.State != "candidate_ready" || v.Snapshot.Candidate == nil || v.Snapshot.Candidate.SkillID != "characterized-skill" {
		t.Fatalf("state = %s, candidate = %+v", v.State, v.Snapshot.Candidate)
	}
	if v.Snapshot.SpentUSD == nil || *v.Snapshot.SpentUSD < 0.2999 || *v.Snapshot.SpentUSD > 0.3001 {
		t.Errorf("spent = %v, want 0.30 including the duplicate check", v.Snapshot.SpentUSD)
	}
	var existing string
	if err := pool.QueryRow(context.Background(), `SELECT snapshot->>'existing_skill_id' FROM creation_sessions WHERE id = $1`, id).Scan(&existing); err != nil {
		t.Fatal(err)
	}
	if existing != "characterized-skill" {
		t.Errorf("existing skill after saving = %q, want characterized-skill", existing)
	}
}

func TestAMaterializedDiagramSessionRecordsTheDiagramAsAnInput(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	probe := &materializeProbe{createdSkill: "diagram-skill"}
	svc := probe.service(pool)
	snapshot := saveReadySnapshot("一份會議紀錄")
	snapshot["diagram_fingerprint"] = "abc123"
	snapshot["diagram_media_type"] = "image/png"
	snapshot["diagram_bytes"] = 3
	snapshot["diagram_description_confirmed"] = true
	snapshot["diagram_confirmed"] = true
	snapshot["diagram_interpretation"] = map[string]any{"nodes": []string{"開始"}, "conditions": []string{}, "branches": []string{}, "uncertainties": []any{}}
	id := seedSaveReadySession(t, pool, svc, ws, snapshot)

	if _, _, err := svc.Act(context.Background(), ws, id, materializeCommand(t)); err != nil {
		t.Fatal(err)
	}
	var inputs struct {
		Diagram *struct {
			SHA256    string `json:"sha256"`
			MediaType string `json:"media_type"`
			Bytes     int    `json:"bytes"`
		} `json:"diagram"`
	}
	if err := json.Unmarshal(probe.provenance.Inputs, &inputs); err != nil {
		t.Fatal(err)
	}
	if inputs.Diagram == nil || inputs.Diagram.SHA256 != "abc123" || inputs.Diagram.MediaType != "image/png" || inputs.Diagram.Bytes != 3 {
		t.Errorf("provenance inputs = %s, want the diagram fingerprint, type and size", probe.provenance.Inputs)
	}
	if probe.testPrompt != "一份會議紀錄" {
		t.Errorf("acceptance test prompt = %q, want the sample input", probe.testPrompt)
	}
}

func TestASessionThatMovedWhileTheSkillWasBeingStoredIsNotSaved(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	removeCreationWorkspace(t, pool, ws)
	probe := &materializeProbe{createdSkill: "raced-skill"}
	svc := probe.service(pool)
	id := seedSaveReadySession(t, pool, svc, ws, saveReadySnapshot("一份會議紀錄"))
	probe.beforeCommit = func(ctx context.Context) {
		if _, err := pool.Exec(ctx, `UPDATE creation_sessions SET revision = revision + 1 WHERE id = $1`, id); err != nil {
			t.Error(err)
		}
	}

	if _, _, err := svc.Act(context.Background(), ws, id, materializeCommand(t)); !errors.Is(err, creation.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	var state string
	if err := pool.QueryRow(context.Background(), `SELECT state FROM creation_sessions WHERE id = $1`, id).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "waiting_input" {
		t.Errorf("state = %s, want the session left as it was", state)
	}
}
