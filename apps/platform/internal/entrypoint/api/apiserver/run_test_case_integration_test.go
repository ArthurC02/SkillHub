package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const savedRunCriteria = `[{"id":"c1","text":"指出缺少日期","source":"suggested","confirmed_at":null},` +
	`{"id":"c2","text":"仍算出總額","source":"user","confirmed_at":"2026-11-01T00:00:00Z"}]`

const savedRunRubric = `{"version":"r1","items":[{"id":"c1","text":"要引用原文","evidence_required":true}]}`

func seedRunToSave(t *testing.T, pool *pgxpool.Pool, workspaceID, name string) (skillID, testCaseID, runID string) {
	t.Helper()
	ctx := context.Background()
	skillID = seedSkill(t, pool, workspaceID, name+"-skill")
	versionID := cmpVersion(t, pool, workspaceID, skillID, name)
	testCaseID = seedTestCase(t, pool, workspaceID, skillID)
	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria,
			content_hash, rubric, simulated_user_facts)
		VALUES ($1, $2, '這張發票沒有日期

你先前提出的問題與我的回答：
1. 問：哪一國的稅？
   答：台灣', $3::jsonb, $4, $5::jsonb, ARRAY['我在台北'])
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID), savedRunCriteria, "sha256:save-"+name, savedRunRubric,
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		WITH r AS (
			INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider, status, finished_at)
			VALUES ($1, $2, $3, 'fake_sandbox', 'succeeded', now())
			RETURNING id, workspace_id),
		s AS (INSERT INTO run_snapshots (run_id, workspace_id) SELECT id, workspace_id FROM r)
		SELECT id::text FROM r`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID),
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	return skillID, testCaseID, runID
}

func saveBody(name string) string { return fmt.Sprintf(`{"name":%q}`, name) }

func TestSavingARunCopiesItsFrozenSnapshotIntoANewDraftOnItsSkill(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "save-run-owner")
	skillID, originalID, runID := seedRunToSave(t, pool, alice.workspaceID, "save-run-copy")
	if _, err := pool.Exec(context.Background(),
		`UPDATE test_cases SET user_prompt = 'edited after the run' WHERE id = $1`, mustUUID(t, originalID)); err != nil {
		t.Fatal(err)
	}

	code, body := alice.doJSON(t, http.MethodPost, "/runs/"+runID+"/test-case", saveBody("  從試跑存的  "))
	if code != http.StatusCreated {
		t.Fatalf("saving a run: got %d (%v), want 201", code, body)
	}
	newID, _ := body["test_case_id"].(string)
	if newID == "" || newID == originalID {
		t.Fatalf("saved test case id = %q, want a new one (original %s)", newID, originalID)
	}
	if body["skill_id"] != skillID || body["name"] != "從試跑存的" {
		t.Errorf("skill %v name %q, want %s and the trimmed name", body["skill_id"], body["name"], skillID)
	}
	if prompt, _ := body["user_prompt"].(string); !strings.HasPrefix(prompt, "這張發票沒有日期") || !strings.Contains(prompt, "答：台灣") {
		t.Errorf("prompt = %q, want the run's frozen prompt with its answers", prompt)
	}
	criteria, _ := json.Marshal(body["acceptance_criteria"])
	if want := `[{"confirmed_at":null,"id":"c1","source":"suggested","text":"指出缺少日期"},` +
		`{"confirmed_at":"2026-11-01T00:00:00Z","id":"c2","source":"user","text":"仍算出總額"}]`; string(criteria) != want {
		t.Errorf("criteria = %s, want %s", criteria, want)
	}
	rubric, _ := json.Marshal(body["rubric"])
	if want := `{"items":[{"evidence_required":true,"id":"c1","text":"要引用原文"}],"version":"r1"}`; string(rubric) != want {
		t.Errorf("rubric = %s, want %s", rubric, want)
	}
	facts, _ := json.Marshal(body["simulated_user_facts"])
	if string(facts) != `["我在台北"]` {
		t.Errorf("simulated user facts = %s, want the snapshot's", facts)
	}
	var originalPrompt string
	if err := pool.QueryRow(context.Background(),
		`SELECT user_prompt FROM test_cases WHERE id = $1`, mustUUID(t, originalID)).Scan(&originalPrompt); err != nil {
		t.Fatal(err)
	}
	if originalPrompt != "edited after the run" {
		t.Errorf("the original draft changed to %q", originalPrompt)
	}
}

func TestSavingAnotherWorkspacesRunIsNotFoundAndCreatesNothing(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "save-run-alice")
	bob := a.login(t, "save-run-bob")
	_, _, runID := seedRunToSave(t, pool, alice.workspaceID, "save-run-stranger")

	if code, body := bob.doJSON(t, http.MethodPost, "/runs/"+runID+"/test-case", saveBody("偷存")); code != http.StatusNotFound {
		t.Errorf("a stranger saving alice's run: got %d (%v), want 404", code, body)
	}
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM test_cases WHERE workspace_id = $1`, mustUUID(t, bob.workspaceID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("bob has %d test cases after a refused save, want 0", n)
	}
}

func TestSavingARunTakesANameAtItsLimitAndRefusesBlankOrOneLonger(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "save-run-name")
	_, _, runID := seedRunToSave(t, pool, alice.workspaceID, "save-run-name")
	path := "/runs/" + runID + "/test-case"

	for _, tc := range []struct {
		name string
		want int
	}{
		{"   ", http.StatusBadRequest},
		{strings.Repeat("名", 201), http.StatusBadRequest},
		{strings.Repeat("名", 200), http.StatusCreated},
	} {
		if code, body := alice.doJSON(t, http.MethodPost, path, saveBody(tc.name)); code != tc.want {
			t.Errorf("name of %d characters: got %d (%v), want %d", len([]rune(tc.name)), code, body, tc.want)
		}
	}
}

func TestSavingARunWhoseSkillWasDeletedIsNotFound(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "save-run-gone")
	skillID, _, runID := seedRunToSave(t, pool, alice.workspaceID, "save-run-gone")
	if _, err := pool.Exec(context.Background(),
		`UPDATE skills SET deleted_at = now() WHERE id = $1`, mustUUID(t, skillID)); err != nil {
		t.Fatal(err)
	}
	if code, body := alice.doJSON(t, http.MethodPost, "/runs/"+runID+"/test-case", saveBody("存不了")); code != http.StatusNotFound {
		t.Errorf("saving a run of a deleted skill: got %d (%v), want 404", code, body)
	}
}
