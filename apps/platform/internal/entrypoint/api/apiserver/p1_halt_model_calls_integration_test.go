package apiserver_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/db/gen"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
)

func TestAP1HaltRefusesGenerationAndCriteriaSuggestionsWithoutCallingTheModel(t *testing.T) {
	pool := requireDB(t)
	clearRunBacklog(t, pool)
	stub := newGenerateStub(t)
	a := newAPIWithLLM(t, pool, stub.URL)
	operator := a.login(t, freshName("operator-p1-model-calls"))
	a.auth.Operators = map[string]bool{operator.userID: true}
	alice := a.login(t, "alice-p1-model-calls")
	_, testCaseID := newTestCase(t, pool, a, alice, "p1-model-calls")

	declareP1Halt(t, operator, "model calls stop with dispatch")
	t.Cleanup(func() {
		if code, body := operatorCall(t, operator, http.MethodDelete, "/admin/dispatch/halt",
			`{"note":"test over"}`); code != http.StatusNoContent {
			t.Errorf("lifting the P1 halt: got %d (%v)", code, body)
		}
	})

	code, body := alice.doJSON(t, http.MethodPost, "/test-cases/"+testCaseID+"/criteria/suggest", "")
	if msg, _ := body["error"].(string); code != http.StatusServiceUnavailable || !strings.Contains(msg, "平台暫停中") {
		t.Errorf("suggesting criteria under a P1 halt: got %d %q, want 503 naming the halt", code, msg)
	}

	_, err := a.versions.GenerateSkill(context.Background(), workspaceOf(t, pool, alice),
		ingest.GenerateInput{TaskDescription: "把 PDF 轉成純文字。"})
	if !errors.Is(err, ingest.ErrModelCallsHalted) {
		t.Errorf("generating under a P1 halt returned %v, want ErrModelCallsHalted", err)
	}
	if stub.calls != 0 {
		t.Errorf("the model was called %d times under a P1 halt", stub.calls)
	}
}

func TestACreationStepUnderAP1HaltSkipsTheModelCallAndWaitsForInput(t *testing.T) {
	pool := requireDB(t)
	ws := newCreationWorkspace(t, pool)
	rec := &jobRecorder{}
	svc := &creation.Service{
		Pool: pool, Limits: creationLimits(), Insert: rec.insert,
		LLM: failLLM(t), IssueKey: failIssueKey(t), RevokeKey: okRevokeKey,
		Billing: creation.BillingHooks{ModelCallsHalted: func(context.Context) bool { return true }},
	}
	id := creationID(t)
	if _, err := svc.Create(context.Background(), ws, id, "開始創作", .5); err != nil {
		t.Fatal(err)
	}
	if err := svc.Step(context.Background(), rec.calls[0], nil); err != nil {
		t.Fatal(err)
	}
	v, err := svc.Get(context.Background(), ws, id)
	if err != nil {
		t.Fatal(err)
	}
	if v.State != string(creation.StateWaitingInput) {
		t.Fatalf("state = %q, want waiting_input so the person can send again after the halt", v.State)
	}
	if v.Snapshot.Steps != 0 {
		t.Errorf("steps = %d, want 0: a step that never called the model is not counted", v.Snapshot.Steps)
	}
	last := v.Snapshot.Messages[len(v.Snapshot.Messages)-1]
	if !strings.Contains(last.Content, "平台暫停中") {
		t.Errorf("last message = %q, want it to name the halt", last.Content)
	}
}

func TestTimeTheQueueSpentHaltedDoesNotCountAgainstTheSlotWaitLimit(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	f := newFixture(t, a, pool, "alice-slot-wait-held")
	ctx := context.Background()
	queued := f.start(t)
	waited := run.SlotWaitLimit + 10*time.Minute
	mustExec(t, pool, `UPDATE runs SET created_at = now() - make_interval(secs => $2) WHERE id = $1`,
		mustUUID(t, queued.RunID), waited.Seconds())
	mustExec(t, pool, `
		INSERT INTO dispatch_halts (provider, source, reason, declared_at, lifted_at, lift_reason)
		VALUES ('', 'orphan_threshold', 'held for the slot-wait test', now() - interval '35 minutes', now() - interval '20 minutes', 'cleared')`)

	if err := (&run.Service{Pool: pool, Gateway: providertest.NewGateway()}).Supervise(ctx); err != nil {
		t.Fatal(err)
	}

	if _, view := f.getRun(t, queued.RunID); view.Status != string(gen.RunStatusQueued) {
		t.Errorf("a run queued %s, fifteen minutes of it under a halt, is %q (%s); want still queued",
			waited, view.Status, view.StatusReason)
	}
}

func TestABatchStartsNoTrialWhileDispatchIsPaused(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	clearRunBacklog(t, pool)
	f := newFixture(t, a, pool, freshName("alice-batch-held"))
	f.confirmPermissions(t)
	var haltID string
	if err := pool.QueryRow(context.Background(), `
		INSERT INTO dispatch_halts (provider, source, reason) VALUES ('', 'orphan_threshold', 'held for the batch test')
		RETURNING id::text`).Scan(&haltID); err != nil {
		t.Fatal(err)
	}
	lift := func() {
		mustExec(t, pool, `UPDATE dispatch_halts SET lifted_at = now(), lift_reason = 'test over' WHERE id = $1 AND lifted_at IS NULL`,
			mustUUID(t, haltID))
	}
	t.Cleanup(lift)

	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches", batchBody(f.versionID, 1, f.testCaseID))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, body)
	}
	batchID := body["id"].(string)
	if got := started(batchTrialRuns(t, pool, batchID)); len(got) != 0 {
		t.Fatalf("a batch created while dispatch is paused started %v, want no trial", got)
	}

	lift()
	if err := a.batches.AdvanceBatches(context.Background(), mustUUID(t, f.workspaceID)); err != nil {
		t.Fatal(err)
	}
	if got := started(batchTrialRuns(t, pool, batchID)); len(got) != 1 {
		t.Errorf("after the halt lifted the batch started %v, want its one trial", got)
	}
}
