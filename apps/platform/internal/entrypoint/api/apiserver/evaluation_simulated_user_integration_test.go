package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/batch"
)

var waterFacts = []string{"本期用水 45 度", "上期沒有欠費"}

func seedFactsAskingRun(t *testing.T, pool *pgxpool.Pool, f fixture, facts []string, questions ...string) string {
	t.Helper()
	runID := seedFinishedRun(t, pool, f, finishedRun{
		testCaseID: f.testCaseID, criteria: asksFirst, contentHash: "sha256:simulated", facts: facts,
	})
	seedQuestions(t, pool, f.workspaceID, runID, questions...)
	return runID
}

func seedBatchByCreator(t *testing.T, pool *pgxpool.Pool, f fixture, runIDs ...string) string {
	t.Helper()
	batchID := seedBatchOfOneTrial(t, pool, f)
	mustExec(t, pool, `UPDATE evaluation_batches SET created_by = $2 WHERE id = $1`,
		mustUUID(t, batchID), mustUUID(t, f.userID))
	for position, runID := range runIDs {
		testCaseID := seedTestCase(t, pool, f.workspaceID, f.skillID)
		mustExec(t, pool, `
			INSERT INTO evaluation_batch_items (batch_id, workspace_id, test_case_id, position, trial, summary_hash, run_id)
			VALUES ($1, $2, $3, $4, 1, 'sha256:summary', $5)`,
			mustUUID(t, batchID), mustUUID(t, f.workspaceID), mustUUID(t, testCaseID), position, mustUUID(t, runID))
	}
	return batchID
}

type continued struct {
	prompt    string
	facts     []string
	answers   []string
	simulated bool
}

func continuationOf(t *testing.T, pool *pgxpool.Pool, runID string) (continued, bool) {
	t.Helper()
	var c continued
	var raw []byte
	err := pool.QueryRow(context.Background(), `
		SELECT s.user_prompt, s.simulated_user_facts, c.answers, c.answered_as_simulated_user
		FROM run_continuations c
		JOIN runs r ON r.id = c.run_id
		JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
		WHERE c.continues_run_id = $1`, mustUUID(t, runID)).Scan(&c.prompt, &c.facts, &raw, &c.simulated)
	if err != nil {
		return continued{}, false
	}
	if err := json.Unmarshal(raw, &c.answers); err != nil {
		t.Fatal(err)
	}
	return c, true
}

func batchSettled(t *testing.T, pool *pgxpool.Pool, batchID string) bool {
	t.Helper()
	var settled bool
	if err := pool.QueryRow(context.Background(),
		`SELECT settled_at IS NOT NULL FROM evaluation_batches WHERE id = $1`, mustUUID(t, batchID)).Scan(&settled); err != nil {
		t.Fatal(err)
	}
	return settled
}

func TestABatchAnswersAQuestionAsTheSimulatedPersonOnlyWhereTheTestCaseHasFacts(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-simulated-user")
	withFacts := seedFactsAskingRun(t, pool, f, waterFacts, "本期用了幾度？", "有沒有欠費？")
	withoutFacts := seedAskingRun(t, pool, f, "本期用了幾度？")
	batchID := seedBatchByCreator(t, pool, f, withoutFacts, withFacts)

	for range 2 {
		if err := a.batches.AdvanceBatches(context.Background(), mustUUID(t, f.workspaceID)); err != nil {
			t.Fatal(err)
		}
	}

	next, found := continuationOf(t, pool, withFacts)
	reply := batch.SimulatedUserReply(waterFacts)
	if !found || !slices.Equal(next.answers, []string{reply, reply}) {
		t.Fatalf("the trial with facts was answered %v (found %v), want both questions answered with %q", next.answers, found, reply)
	}
	if !next.simulated {
		t.Error("the continuation the batch answered is not marked as answered by the simulated person")
	}
	if !strings.Contains(next.prompt, "本期用水 45 度") {
		t.Errorf("the continuing run's prompt %q does not carry the simulated person's facts", next.prompt)
	}
	if !slices.Equal(next.facts, waterFacts) {
		t.Errorf("the continuing run froze facts %q, want the simulated person carried into the next round", next.facts)
	}
	if unanswered, found := continuationOf(t, pool, withoutFacts); found {
		t.Errorf("the trial without facts was answered %v, want it left for a real person", unanswered.answers)
	}
	if batchSettled(t, pool, batchID) {
		t.Error("the batch settled while the answered trial's next run has not ended")
	}
}

func TestOneBatchFailingToAnswerLeavesTheOtherBatchesToSettle(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-simulated-isolated")
	failing := seedBatchByCreator(t, pool, f, seedFactsAskingRun(t, pool, f, waterFacts, "本期用了幾度？"))
	mustExec(t, pool, `UPDATE evaluation_batches SET created_at = now() - interval '1 minute' WHERE id = $1`, mustUUID(t, failing))
	quiet := seedBatchByCreator(t, pool, f, seedAskingRun(t, pool, f, "本期用了幾度？"))

	broken := errors.New("connection reset")
	isolated := *a.batches
	isolated.Ports.AnswerRun = func(context.Context, batch.BatchAnswer) error { return broken }
	if err := isolated.AdvanceBatches(context.Background(), mustUUID(t, f.workspaceID)); !errors.Is(err, broken) {
		t.Fatalf("advance = %v, want the failed answer returned so the round is retried", err)
	}
	if batchSettled(t, pool, failing) {
		t.Error("the batch whose answer failed settled")
	}
	if !batchSettled(t, pool, quiet) {
		t.Error("the batch that owed no answer did not settle because another batch's answer failed")
	}
}

func TestABatchSettlesOnceNoTrialIsRunningAndNoSimulatedAnswerIsOwed(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	resetRunSettings(t, pool)
	operator := a.login(t, freshName("operator-simulated-rounds"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	if code, body := operatorCall(t, operator, http.MethodPut, "/admin/settings/runs/continuation-rounds",
		`{"rounds":1,"note":"one round for the simulated person"}`); code != http.StatusOK {
		t.Fatalf("set rounds: %d %v", code, body)
	}
	f := newFixture(t, a, pool, "alice-simulated-settle")
	withoutFacts := seedAskingRun(t, pool, f, "本期用了幾度？")
	firstRound := seedFactsAskingRun(t, pool, f, waterFacts, "本期用了幾度？")
	atLimit := seedFactsAskingRun(t, pool, f, waterFacts, "還有別的嗎？")
	linkContinuation(t, pool, f.workspaceID, firstRound, atLimit)
	batchID := seedBatchByCreator(t, pool, f, withoutFacts, firstRound)

	if err := a.batches.AdvanceBatches(context.Background(), mustUUID(t, f.workspaceID)); err != nil {
		t.Fatal(err)
	}

	if past, found := continuationOf(t, pool, atLimit); found {
		t.Errorf("the conversation past its round limit was answered %v", past.answers)
	}
	if !batchSettled(t, pool, batchID) {
		t.Error("the batch did not settle although every trial ended and no answer can be given")
	}
}

func TestTheSimulatedPersonIsEditedOnTheTestCaseAndFrozenIntoItsSnapshot(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	alice := a.login(t, "alice-simulated-facts")
	_, id := newTestCase(t, pool, a, alice, "simulated-facts")

	code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"simulated_user_facts":[" 本期用水 45 度 ","上期沒有欠費"]}`)
	if code != http.StatusOK || !sameFacts(body["simulated_user_facts"], waterFacts) {
		t.Fatalf("PATCH facts: %d %v, want %q stored trimmed", code, body["simulated_user_facts"], waterFacts)
	}
	if code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"simulated_user_facts":["  "]}`); code != http.StatusBadRequest {
		t.Errorf("a blank fact: %d %v, want 400", code, body)
	}
	if code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"name":"renamed"}`); code != http.StatusOK ||
		!sameFacts(body["simulated_user_facts"], waterFacts) {
		t.Errorf("an edit without facts left %v, want the stored facts kept", body["simulated_user_facts"])
	}

	snap := takeSnapshot(t, pool, mustUUID(t, alice.workspaceID), mustUUID(t, id))
	var frozen []string
	if err := pool.QueryRow(context.Background(),
		`SELECT simulated_user_facts FROM test_case_snapshots WHERE id = $1`, snap.ID).Scan(&frozen); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(frozen, waterFacts) {
		t.Errorf("the snapshot froze %q, want %q", frozen, waterFacts)
	}

	if code, body := alice.doJSON(t, http.MethodPatch, "/test-cases/"+id, `{"simulated_user_facts":[]}`); code != http.StatusOK ||
		!sameFacts(body["simulated_user_facts"], nil) {
		t.Errorf("clearing the facts: %d %v, want an empty list", code, body["simulated_user_facts"])
	}
}

func sameFacts(got any, want []string) bool {
	list, ok := got.([]any)
	if !ok || len(list) != len(want) {
		return false
	}
	for i, fact := range list {
		if fact != want[i] {
			return false
		}
	}
	return true
}
