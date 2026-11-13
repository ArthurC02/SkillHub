package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/jackc/pgx/v5"
)

func removeCreationEvaluationCases(t *testing.T, service *creation.Service, creator *client, view creation.View) creation.View {
	t.Helper()
	if _, err := testPool.Exec(context.Background(), `UPDATE creation_sessions
		SET snapshot = jsonb_set(snapshot, '{snapshot,challenge_cases}', '[]'::jsonb)
		WHERE id = $1 AND workspace_id = $2`, mustUUID(t, view.ID), mustUUID(t, creator.workspaceID)); err != nil {
		t.Fatal(err)
	}
	current, err := service.Get(context.Background(), identity.Workspace{ID: mustUUID(t, creator.workspaceID)}, mustUUID(t, view.ID))
	if err != nil {
		t.Fatal(err)
	}
	return current
}

func assertCreationRepairsEvaluationCases(t *testing.T, view creation.View) {
	t.Helper()
	if view.State != "queued" || view.Snapshot.BriefConfirmed || view.Snapshot.Candidate != nil {
		t.Fatalf("missing cases were accepted: %+v", view)
	}
	messages := view.Snapshot.Messages
	if len(messages) == 0 || !strings.Contains(messages[len(messages)-1].Content, "評估案例") {
		t.Fatalf("missing cases were not explained: %+v", view)
	}
}

func TestCreationRepairsLegacyCasesBeforeConfirmationAndNewCandidate(t *testing.T) {
	a, service, calls := creationFixture(t)
	creator := a.login(t, "creation-legacy-evaluation-pair")
	view := creationPost(t, creator, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650,
	}, http.StatusOK)
	view = creationStep(t, service, view)
	view = removeCreationEvaluationCases(t, service, creator, view)
	view = creationAct(t, creator, view, "confirm_brief")
	assertCreationRepairsEvaluationCases(t, view)
	view = creationStep(t, service, view)
	if view.State != "waiting_confirmation" || len(view.Snapshot.ChallengeCases) != 2 {
		t.Fatalf("legacy brief did not return for explicit confirmation: %+v", view)
	}
	view = creationAct(t, creator, view, "confirm_brief")
	view = creationStep(t, service, view)
	if view.Snapshot.Draft == nil {
		t.Fatalf("confirmed pair did not produce a draft: %+v", view)
	}
	view = removeCreationEvaluationCases(t, service, creator, view)
	view = creationAct(t, creator, view, "materialize")
	assertCreationRepairsEvaluationCases(t, view)
	var skills int
	if err := testPool.QueryRow(context.Background(), "SELECT count(*) FROM skills WHERE workspace_id = $1", mustUUID(t, creator.workspaceID)).Scan(&skills); err != nil {
		t.Fatal(err)
	}
	if skills != 0 {
		t.Fatalf("legacy draft created %d Skills before its cases were confirmed", skills)
	}
	view = creationStep(t, service, view)
	if view.State != "waiting_confirmation" || view.Snapshot.BriefConfirmed || len(view.Snapshot.ChallengeCases) != 2 {
		t.Fatalf("repaired draft cases were not put to the author: %+v", view)
	}
	view = creationAct(t, creator, view, "confirm_brief")
	view = creationStep(t, service, view)
	view = creationAct(t, creator, view, "materialize")
	if view.Snapshot.Candidate == nil || len(view.Snapshot.Candidate.ChallengeTestCaseIDs) != 2 || calls.Load() != 5 {
		t.Fatalf("confirmed cases were not materialized: view=%+v calls=%d", view, calls.Load())
	}
}

func TestCreationRollsBackCandidateWhenAnEvaluationCaseCannotBeWritten(t *testing.T) {
	for _, tc := range []struct {
		name   string
		failAt int
	}{{"first_evaluation", 2}, {"second_evaluation", 3}} {
		t.Run(tc.name, func(t *testing.T) {
			assertCreationEvaluationWriteRollback(t, tc.name, tc.failAt)
		})
	}
}

func assertCreationEvaluationWriteRollback(t *testing.T, name string, failAt int) {
	t.Helper()
	a, service, _ := creationFixture(t)
	creator := a.login(t, "creation-evaluation-case-rollback-"+name)
	view := creationPost(t, creator, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": "請建立資料摘要 Skill。", "budget_credits": 650,
	}, http.StatusOK)
	view = creationStep(t, service, view)
	view = creationAct(t, creator, view, "confirm_brief")
	view = creationStep(t, service, view)
	write := a.app.CreationSvc.CreateAcceptanceTestCase
	if write == nil {
		t.Fatal("the API did not wire the Test Case writer")
	}
	writes := 0
	a.app.CreationSvc.CreateAcceptanceTestCase = func(ctx context.Context, tx pgx.Tx, ws identity.Workspace, skillID, name, prompt string, criteria []string) (string, error) {
		writes++
		if writes == failAt {
			return "", errors.New("fixture evaluation case write failed")
		}
		return write(ctx, tx, ws, skillID, name, prompt, criteria)
	}
	status, body := creationPostStatus(t, creator, "/creation-sessions/"+view.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": view.Revision,
		"kind": "materialize", "content_hash": view.Snapshot.Draft.ContentHash,
	})
	if status != http.StatusServiceUnavailable || writes != failAt {
		t.Fatalf("materialize status=%d writes=%d body=%s, want the failed evaluation write", status, writes, body)
	}
	current, err := service.Get(context.Background(), identity.Workspace{ID: mustUUID(t, creator.workspaceID)}, mustUUID(t, view.ID))
	if err != nil {
		t.Fatal(err)
	}
	if current.Revision != view.Revision || current.Snapshot.Candidate != nil {
		t.Fatalf("failed case write advanced the session: %+v", current)
	}
	var skills, cases int
	if err := testPool.QueryRow(context.Background(), `SELECT
		(SELECT count(*) FROM skills WHERE workspace_id = $1),
		(SELECT count(*) FROM test_cases WHERE workspace_id = $1)`, mustUUID(t, creator.workspaceID)).Scan(&skills, &cases); err != nil {
		t.Fatal(err)
	}
	if skills != 0 || cases != 0 {
		t.Fatalf("failed case write left %d Skills and %d Test Cases", skills, cases)
	}
}
