// Evaluation end to end through the real route table (EVAL-001, ADR-025,
// ADR-026). The judge is an httptest server speaking llm-internal.yaml, because
// what is under test is the platform's half: the append-only revision chain, the
// two trace events, workspace scope, and the promise that a verdict never moves a
// run.
package apiserver_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/ArthurC02/skillhub/apps/platform/internal/eval"
	"github.com/ArthurC02/skillhub/apps/platform/internal/llmclient"
)

// --- fixtures ----------------------------------------------------------------

// seedEvaluatableRun writes a finished run with two acceptance criteria in its
// snapshot, one artifact, and a final agent output to quote from.
// The optional rubric is the frozen CONTENT-007 one, as JSON; omitted means the
// snapshot has none, which is what all 45 M2 baseline snapshots look like.
func seedEvaluatableRun(t *testing.T, pool *pgxpool.Pool, workspaceID, skillID string, rubric ...string) (runID, versionID string) {
	t.Helper()
	ctx := context.Background()
	versionID = seedSkillVersion(t, pool, workspaceID, skillID)
	testCaseID := seedTestCase(t, pool, workspaceID, skillID)

	var frozenRubric *string
	if len(rubric) == 1 {
		frozenRubric = &rubric[0]
	}
	var snapshotID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO test_case_snapshots (workspace_id, test_case_id, user_prompt, acceptance_criteria, content_hash, rubric)
		VALUES ($1, $2, 'deduplicate the attached spreadsheet',
		        '[{"id":"c1","text":"duplicates are removed","source":"user"},
		          {"id":"c2","text":"an xlsx file is produced","source":"user"}]'::jsonb,
		        'sha256:eval-snapshot', $3::jsonb)
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, testCaseID), frozenRubric,
	).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}

	if err := pool.QueryRow(ctx, `
		INSERT INTO runs (workspace_id, skill_version_id, test_case_snapshot_id, provider,
		                  runtime_snapshot, policy_snapshot, status, finished_at)
		VALUES ($1, $2, $3, 'fake_sandbox', '{}'::jsonb, '{}'::jsonb, 'succeeded', now())
		RETURNING id::text`,
		mustUUID(t, workspaceID), mustUUID(t, versionID), mustUUID(t, snapshotID),
	).Scan(&runID); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		INSERT INTO artifacts (workspace_id, run_id, kind, file_name, content_type,
		                       size_bytes, content_hash, object_key, expires_at)
		VALUES ($1, $2, 'run_output', 'output.xlsx', 'application/vnd.ms-excel',
		        4096, 'sha256:output', 'run-artifacts/x/y/artifacts.tar', now() + interval '30 days')`,
		mustUUID(t, workspaceID), mustUUID(t, runID),
	); err != nil {
		t.Fatal(err)
	}
	return runID, versionID
}

// seedFinalOutput puts one agent_output event on the run, which is what an
// agent_output evidence reference is verified against.
func seedFinalOutput(t *testing.T, pool *pgxpool.Pool, workspaceID, runID, text string) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"kind": "final", "text": text, "truncated": false})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO trace_events (event_id, workspace_id, run_id, attempt, seq, occurred_at, event_type,
		                          source, status, schema_version, masked, masked_fields, payload)
		VALUES (gen_random_uuid(), $1, $2, 1, 1, now(), 'agent_output', 'llm_service', 'ok',
		        '1.0', true, '[]'::jsonb, $3)`,
		mustUUID(t, workspaceID), mustUUID(t, runID), payload,
	); err != nil {
		t.Fatal(err)
	}
}

// judgeServer answers /judge-run with whatever the test hands it.
func judgeServer(t *testing.T, verdict llmclient.JudgeVerdict, promptVersion string) *llmclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: verdict, Model: "gpt-5.6-terra", PromptVersion: promptVersion,
			Usage: &llmclient.GatewayUsage{PromptTokens: 11, CompletionTokens: 7},
		})
	}))
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

type evaluationBody struct {
	EvaluationID     string `json:"evaluation_id"`
	RunID            string `json:"run_id"`
	Status           string `json:"status"`
	Overall          string `json:"overall"`
	Summary          string `json:"summary"`
	CriterionResults []struct {
		CriterionID string `json:"criterion_id"`
		Text        string `json:"text"`
		Result      string `json:"result"`
		Source      string `json:"source"`
		Reason      string `json:"reason"`
		Evidence    []struct {
			Kind      string `json:"kind"`
			Excerpt   string `json:"excerpt"`
			Available bool   `json:"available"`
		} `json:"evidence"`
	} `json:"criterion_results"`
	DeterministicFindings []struct {
		Category string `json:"category"`
		Severity string `json:"severity"`
		Message  string `json:"message"`
	} `json:"deterministic_findings"`
	JudgeModel         string  `json:"judge_model"`
	JudgePromptVersion string  `json:"judge_prompt_version"`
	RubricVersion      string  `json:"rubric_version"`
	EvidenceComplete   bool    `json:"evidence_complete"`
	SupersededAt       *string `json:"superseded_at"`
	Cost               struct {
		EvaluationUSD *float64 `json:"evaluation_usd"`
		Source        string   `json:"source"`
		Note          string   `json:"note"`
	} `json:"cost"`
	Feedback *struct {
		Helpful bool   `json:"helpful"`
		Comment string `json:"comment"`
	} `json:"feedback"`
	Error string `json:"error"`
}

func (c *client) getEvaluation(t *testing.T, path string) (int, evaluationBody) {
	t.Helper()
	resp, err := c.Get(c.base + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body evaluationBody
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body
}

func TestEvaluationRetryDoesNotRepeatACompletedJudgeCall(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-recovery")
	skillID := seedSkill(t, pool, c.workspaceID, "eval-recovery")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)

	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: llmclient.JudgeVerdict{Overall: "undetermined", CriterionResults: []llmclient.CriterionVerdict{
				{CriterionID: "c1", Result: "undetermined", Reason: "not enough evidence"},
				{CriterionID: "c2", Result: "undetermined", Reason: "not enough evidence"},
			}}, Model: "gpt-5.6-terra", PromptVersion: "recovery-test",
		})
	}))
	defer srv.Close()
	a.evaluations.Judge = &llmclient.Client{BaseURL: srv.URL}

	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	worker := &eval.Worker{Svc: a.evaluations}
	err := worker.Work(context.Background(), &river.Job[eval.JobArgs]{
		JobRow: &rivertype.JobRow{Attempt: 2},
		Args:   eval.JobArgs{RunID: runID, WorkspaceID: c.workspaceID},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("judge calls = %d after recovery, want 1", got)
	}
	var evaluations int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM evaluations WHERE run_id = $1", mustUUID(t, runID)).Scan(&evaluations); err != nil {
		t.Fatal(err)
	}
	if evaluations != 1 {
		t.Fatalf("evaluation revisions = %d after recovery, want 1", evaluations)
	}
}

func TestStalePendingEvaluationIsReconciledWithoutJudge(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-stale-recovery")
	skillID := seedSkill(t, pool, c.workspaceID, "eval-stale-recovery")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete, created_at)
		VALUES ($1, $2, 'pending', 'undetermined', false, now() - interval '20 minutes')`,
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}

	worker := &eval.RecoveryWorker{Svc: a.evaluations}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			errs <- worker.Work(context.Background(), &river.Job[eval.RecoveryArgs]{})
		}()
	}
	close(start)
	for range 2 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	var status string
	if err := pool.QueryRow(context.Background(),
		"SELECT status FROM evaluations WHERE run_id = $1 AND superseded_at IS NULL",
		mustUUID(t, runID)).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("stale evaluation status = %q, want failed", status)
	}
	var completionEvents int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM trace_events
		WHERE run_id = $1 AND event_type = 'evaluation_completed'
		  AND status = 'error'`, mustUUID(t, runID)).Scan(&completionEvents); err != nil {
		t.Fatal(err)
	}
	if completionEvents != 1 {
		t.Fatalf("concurrent recovery wrote %d completion events, want 1", completionEvents)
	}
}

// --- the happy path -----------------------------------------------------------

func TestEvaluationIsRecordedWithVerifiedEvidenceAndNeverTouchesTheRun(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-owner")
	skillID := seedSkill(t, pool, c.workspaceID, "dedupe")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	const finalText = "Removed 17 duplicate rows and saved the result to output.xlsx."
	seedFinalOutput(t, pool, c.workspaceID, runID, finalText)

	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "the reply says 17 rows were removed",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed 17 duplicate rows"},
				}},
			{CriterionID: "c2", Result: "passed", Reason: "output.xlsx is in the manifest",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")},
				}},
		},
		Overall: "met", Summary: "both conditions were met",
	}, "judge-run@2026-08-17")

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	status, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if status != http.StatusOK {
		t.Fatalf("GET evaluation: got %d (%s)", status, body.Error)
	}
	if body.Status != "completed" || body.Overall != "met" {
		t.Errorf("expected a completed, met verdict, got status=%q overall=%q", body.Status, body.Overall)
	}
	if len(body.CriterionResults) != 2 {
		t.Fatalf("one entry per snapshot criterion, got %d", len(body.CriterionResults))
	}
	for _, r := range body.CriterionResults {
		if r.Source != "model" {
			t.Errorf("criterion %s must be labelled a model judgement (EVAL-001 clause 5), got %q", r.CriterionID, r.Source)
		}
		if r.Text == "" {
			t.Errorf("criterion %s carries the snapshot wording so the report reads on its own", r.CriterionID)
		}
		if len(r.Evidence) == 0 {
			t.Errorf("criterion %s passed with no evidence stored", r.CriterionID)
		}
	}
	if body.JudgeModel != "gpt-5.6-terra" || body.JudgePromptVersion != "judge-run@2026-08-17" {
		t.Errorf("the row records what actually judged, got %q / %q", body.JudgeModel, body.JudgePromptVersion)
	}
	if body.Cost.EvaluationUSD != nil {
		t.Error("this judge reported no spend, and an unreported cost is not a number")
	}
	if !strings.Contains(body.Cost.Note, "ADR-017") {
		t.Errorf("the cost note has to name the authoritative source, got %q", body.Cost.Note)
	}

	// The six classes are separated (EVAL-001 clause 1) and the rule leg produced
	// its own findings, apart from the criterion verdicts.
	seen := map[string]bool{}
	for _, f := range body.DeterministicFindings {
		seen[f.Category] = true
	}
	for _, want := range []string{"spec", "activation", "execution", "compatibility", "cost"} {
		if !seen[want] {
			t.Errorf("no deterministic finding for the %q class", want)
		}
	}

	// ADR-025: the run is untouched. Both the status and its reason.
	_, runBody := c.getRun(t, runID)
	if runBody.Status != "succeeded" || runBody.FailureClass != "" {
		t.Errorf("an evaluation must not write back to the run, got %+v", runBody)
	}

	// 丙-2: the timeline does not stop when the run ends, and the two events
	// declare the revision that introduced them.
	assertEvaluationTraceEvents(t, pool, runID, "ok")
}

func assertEvaluationTraceEvents(t *testing.T, pool *pgxpool.Pool, runID, wantStatus string) {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT event_type, source, schema_version, coalesce(status, '')
		FROM trace_events
		WHERE run_id = $1 AND event_type IN ('evaluation_started', 'evaluation_completed')
		ORDER BY seq`, mustUUID(t, runID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var eventType, source, version, status string
		if err := rows.Scan(&eventType, &source, &version, &status); err != nil {
			t.Fatal(err)
		}
		if source != "orchestrator" {
			t.Errorf("%s must be emitted by the orchestrator, got %q", eventType, source)
		}
		if version != "1.2" {
			t.Errorf("%s declares the revision that introduced it, got %q", eventType, version)
		}
		got[eventType] = status
	}
	if _, ok := got["evaluation_started"]; !ok {
		t.Error("no evaluation_started event")
	}
	if status, ok := got["evaluation_completed"]; !ok {
		t.Error("no evaluation_completed event")
	} else if status != wantStatus {
		t.Errorf("evaluation_completed status = %q, want %q", status, wantStatus)
	}
}

func TestEvaluationEventsUseTheLatestPersistedAttempt(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-latest-attempt")
	skillID := seedSkill(t, pool, c.workspaceID, "eval-latest-attempt")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO run_attempts (run_id, workspace_id, attempt_number, provider)
		VALUES ($1, $2, 1, 'fake_sandbox'), ($1, $2, 2, 'fake_sandbox')`,
		mustUUID(t, runID), mustUUID(t, c.workspaceID)); err != nil {
		t.Fatal(err)
	}
	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "undetermined", Reason: "no evidence"},
			{CriterionID: "c2", Result: "undetermined", Reason: "no evidence"},
		},
		Overall: "undetermined", Summary: "no evidence",
	}, "judge-run@latest-attempt")
	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	var minAttempt, maxAttempt int
	if err := pool.QueryRow(context.Background(), `
		SELECT min(attempt), max(attempt) FROM trace_events
		WHERE run_id = $1 AND event_type IN ('evaluation_started', 'evaluation_completed')`,
		mustUUID(t, runID)).Scan(&minAttempt, &maxAttempt); err != nil {
		t.Fatal(err)
	}
	if minAttempt != 2 || maxAttempt != 2 {
		t.Fatalf("evaluation event attempts = %d..%d, want latest persisted attempt 2", minAttempt, maxAttempt)
	}
}

// --- CONTENT-007: the rubric on the product path ------------------------------

// capturingJudgeServer is judgeServer plus a copy of what was actually sent.
func capturingJudgeServer(t *testing.T, verdict llmclient.JudgeVerdict, capture *llmclient.JudgeRunRequest) *llmclient.Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(capture); err != nil {
			t.Errorf("request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(llmclient.JudgeRunResponse{
			Verdict: verdict, Model: "gpt-5.6-terra", PromptVersion: "judge-run@2026-08-17",
		})
	}))
	t.Cleanup(srv.Close)
	return &llmclient.Client{BaseURL: srv.URL}
}

func rubricVersionOfStartedEvent(t *testing.T, pool *pgxpool.Pool, runID string) (any, bool) {
	t.Helper()
	var payload []byte
	if err := pool.QueryRow(context.Background(), `
		SELECT payload FROM trace_events
		WHERE run_id = $1 AND event_type = 'evaluation_started'
		ORDER BY seq DESC LIMIT 1`, mustUUID(t, runID)).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	decoded := map[string]any{}
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	v, present := decoded["rubric_version"]
	return v, present
}

// The whole of G2: the frozen rubric reaches the judge, its version is declared
// on the started event and recorded on the row, and an item naming a criterion
// this run does not have is dropped and said so rather than silently ignored.
func TestTheFrozenRubricReachesTheJudgeAndItsVersionIsRecorded(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-rubric")
	skillID := seedSkill(t, pool, c.workspaceID, "rubric-run")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID, `{
		"version": "content-007/writing/v1",
		"items": [
		  {"id": "c1", "text": "Quote the sentence that shows the duplicates went.", "weight": 3, "evidence_required": true},
		  {"id": "orphan", "text": "strengthens a criterion this snapshot does not carry", "evidence_required": false}
		]}`)
	const finalText = "Removed 17 duplicate rows and saved the result to output.xlsx."
	seedFinalOutput(t, pool, c.workspaceID, runID, finalText)

	var sent llmclient.JudgeRunRequest
	a.evaluations.Judge = capturingJudgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "the reply says 17 rows were removed",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed 17 duplicate rows"},
				}},
			{CriterionID: "c2", Result: "passed", Reason: "output.xlsx is in the manifest",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")},
				}},
		},
		Overall: "met", Summary: "both conditions were met",
	}, &sent)

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	if sent.Rubric == nil {
		t.Fatal("the snapshot's rubric never reached the judge")
	}
	if len(sent.Rubric.Items) != 1 || sent.Rubric.Items[0].ID != "c1" {
		t.Fatalf("only items naming a sent criterion go out, got %+v", sent.Rubric.Items)
	}
	if sent.Rubric.Items[0].Weight == nil || *sent.Rubric.Items[0].Weight != 3 {
		t.Errorf("the item's weight is carried through, got %+v", sent.Rubric.Items[0])
	}

	if got, present := rubricVersionOfStartedEvent(t, pool, runID); !present || got != "content-007/writing/v1" {
		t.Errorf("evaluation_started declares the rubric in force, got %#v (present=%v)", got, present)
	}

	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if body.RubricVersion != "content-007/writing/v1" {
		t.Errorf("the row records the rubric the verdict was reached under, got %q", body.RubricVersion)
	}
	var warned bool
	for _, f := range body.DeterministicFindings {
		if f.Severity == "warning" && strings.Contains(f.Message, "orphan") {
			warned = true
		}
	}
	if !warned {
		t.Errorf("a rubric item nothing will answer has to be said out loud, got %+v", body.DeterministicFindings)
	}
}

// The 45 baseline snapshots have no rubric, and must keep saying so: absent is
// "no rubric", never "the default rubric".
func TestARunWithoutARubricRecordsNoneAtAll(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-no-rubric")
	skillID := seedSkill(t, pool, c.workspaceID, "no-rubric-run")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows.")

	var sent llmclient.JudgeRunRequest
	a.evaluations.Judge = capturingJudgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "failed", Reason: "nothing shows it"},
			{CriterionID: "c2", Result: "failed", Reason: "no file"},
		},
		Overall: "not_met", Summary: "neither condition was met",
	}, &sent)

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if sent.Rubric != nil {
		t.Errorf("no rubric means no rubric field on the request, got %+v", sent.Rubric)
	}
	if got, present := rubricVersionOfStartedEvent(t, pool, runID); !present || got != nil {
		t.Errorf("the started event says null, got %#v (present=%v)", got, present)
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if body.RubricVersion != "" {
		t.Errorf("no rubric was in force, got %q", body.RubricVersion)
	}
}

// --- the downgrade rule, end to end -------------------------------------------

// ADR-026 decision 2: whether a citation still resolves is answered when the
// report is read, never trusted from a flag written when the evidence was cited.
// Retention on trace_events is a partition drop, so a stored `available: true`
// goes on claiming the original is there long after the month it lived in was
// dropped — and the reader is told the excerpt is all that is left rather than
// having it blanked out (ADR-009).
//
// Nothing in this repository asserted `available` before DDD-033. It is now the
// behavioural guard on trace's read face: the report calls trace.LiveEvents for
// this answer, and a read face that returned nothing would label a live citation
// stale, while one that ignored its run or workspace scope would leave this
// citation claiming to resolve after its event is gone.
func TestCitedTraceEvidenceStopsClaimingToResolveOnceItsEventIsGone(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-evidence-availability")
	skillID := seedSkill(t, pool, c.workspaceID, "evidence-availability")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	eventID := seedToolCallEvent(t, pool, c.workspaceID, runID, 7, "deduplicate")

	cite := []llmclient.JudgeEvidenceRef{
		{Kind: "trace_event", TraceEventID: strPtrTest(uuidText(eventID)), Quote: `"tool_name": "deduplicate"`},
	}
	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "the tool call is in the trace", EvidenceRefs: cite},
			{CriterionID: "c2", Result: "passed", Reason: "same call produced the file", EvidenceRefs: cite},
		},
		Overall: "met", Summary: "both conditions were met",
	}, "judge-run@2026-08-17")

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	excerpts := map[string]string{}
	for _, r := range body.CriterionResults {
		var cited bool
		for _, e := range r.Evidence {
			if e.Kind != "trace_event" {
				continue
			}
			cited = true
			if !e.Available {
				t.Errorf("criterion %s cites an event that is still in the table and is labelled unavailable", r.CriterionID)
			}
			excerpts[r.CriterionID] = e.Excerpt
		}
		if !cited {
			t.Fatalf("criterion %s kept no trace_event citation, so this test proves nothing", r.CriterionID)
		}
	}

	// TRUNCATE and not DELETE: 0005's trace_events_immutable trigger is BEFORE
	// UPDATE OR DELETE FOR EACH ROW, which is the execution record being the
	// execution record. Dropping the table's contents is the test harness standing
	// in for the partition drop retention actually performs.
	if _, err := pool.Exec(context.Background(), `TRUNCATE trace_events`); err != nil {
		t.Fatal(err)
	}

	_, after := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if len(after.CriterionResults) != 2 {
		t.Fatalf("the report itself must survive its evidence, got %d criteria", len(after.CriterionResults))
	}
	for _, r := range after.CriterionResults {
		for _, e := range r.Evidence {
			if e.Kind != "trace_event" {
				continue
			}
			if e.Available {
				t.Errorf("criterion %s still claims its citation resolves after the event was dropped", r.CriterionID)
			}
			if e.Excerpt != excerpts[r.CriterionID] {
				t.Errorf("criterion %s lost its excerpt when the event went; a stale citation keeps it, labelled (ADR-009): %q",
					r.CriterionID, e.Excerpt)
			}
		}
	}
	// The verdict itself is untouched: availability is a statement about the
	// evidence, not a re-judgement (iron rule 4 — the evaluation is a record).
	if after.Overall != body.Overall {
		t.Errorf("overall changed from %q to %q when evidence expired", body.Overall, after.Overall)
	}
}

func TestAVerdictOnEvidenceThePlatformCannotFindIsDowngraded(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-downgrade")
	skillID := seedSkill(t, pool, c.workspaceID, "downgrade")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	seedFinalOutput(t, pool, c.workspaceID, runID, "I have finished.")

	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "it says so",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "trace_event", TraceEventID: strPtrTest("11111111-1111-4111-8111-111111111111"),
						Quote: "everything worked"},
				}},
			// c2 is not answered at all: the Python side does not pad the list.
		},
		Overall: "met", Summary: "all good",
	}, "judge-run@2026-08-17")

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("evaluate: %v", err)
	}

	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if len(body.CriterionResults) != 2 {
		t.Fatalf("both criteria are listed whatever the judge answered, got %d", len(body.CriterionResults))
	}
	if body.CriterionResults[0].Result != "undetermined" ||
		!strings.HasPrefix(body.CriterionResults[0].Reason, "evidence_unverifiable:") {
		t.Errorf("an unresolvable citation downgrades the verdict, got %+v", body.CriterionResults[0])
	}
	if body.CriterionResults[1].Result != "undetermined" {
		t.Errorf("a criterion the judge skipped is undetermined, got %+v", body.CriterionResults[1])
	}
	// Go recomputes the overall after downgrading; the model said `met`.
	if body.Overall != "undetermined" {
		t.Errorf("the stored overall is recomputed after the downgrades, got %q", body.Overall)
	}
}

// --- failure is a state, not an absence ---------------------------------------

func TestAJudgeFailureIsRecordedAsAFailedEvaluation(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-judge-down")
	skillID := seedSkill(t, pool, c.workspaceID, "judge-down")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"error":"gateway unavailable"}`, http.StatusBadGateway)
	}))
	defer srv.Close()
	a.evaluations.Judge = &llmclient.Client{BaseURL: srv.URL}

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatalf("a judge failure is an outcome the job records, not an error it raises: %v", err)
	}

	status, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if status != http.StatusOK {
		t.Fatalf("a failed evaluation still has a body: got %d", status)
	}
	if body.Status != "failed" {
		t.Errorf("status must say the evaluation broke, got %q", body.Status)
	}
	if body.Overall != "undetermined" {
		t.Errorf("a failed evaluation reaches no verdict, got %q", body.Overall)
	}
	if len(body.DeterministicFindings) == 0 {
		t.Error("the rule findings came from the platform's own records and survive a judge failure")
	}
	// ADR-026 decision 1 on the path that needs it most. "Which judge could not
	// answer" is the first question asked about a failure, and its only other home
	// is the `evaluation_started` event below — which stops answering the month
	// retention drops that partition.
	if body.JudgeModel != "gpt-5.6-terra" {
		t.Errorf("a failed evaluation must record the judge it was attempted with, got %q", body.JudgeModel)
	}
	assertEvaluationTraceEvents(t, pool, runID, "error")

	// ADR-025 again: even a broken evaluation leaves the run alone.
	_, runBody := c.getRun(t, runID)
	if runBody.Status != "succeeded" {
		t.Errorf("run status changed to %q", runBody.Status)
	}
}

// --- re-evaluation is append-only (ADR-026 decision 1) ------------------------

func TestReEvaluationSupersedesWithoutOverwriting(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-reassess")
	skillID := seedSkill(t, pool, c.workspaceID, "reassess")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	seedFinalOutput(t, pool, c.workspaceID, runID, "Removed 17 duplicate rows.")

	pass := llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "first rubric",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "agent_output", Quote: "Removed 17 duplicate rows"},
				}},
			{CriterionID: "c2", Result: "passed", Reason: "first rubric",
				EvidenceRefs: []llmclient.JudgeEvidenceRef{
					{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")},
				}},
		},
		Overall: "met", Summary: "met under the first rubric",
	}
	a.evaluations.Judge = judgeServer(t, pass, "judge-run@v1")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	_, first := c.getEvaluation(t, "/runs/"+runID+"/evaluation")

	// A stricter prompt, and a different answer.
	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "failed", Reason: "second rubric is stricter", EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "agent_output", Quote: "Removed 17 duplicate rows"}}},
			{CriterionID: "c2", Result: "failed", Reason: "second rubric is stricter", EvidenceRefs: []llmclient.JudgeEvidenceRef{{Kind: "artifact", ArtifactPath: strPtrTest("output.xlsx")}}},
		},
		Overall: "not_met", Summary: "not met under the second rubric",
	}, "judge-run@v2")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}

	_, current := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if current.EvaluationID == first.EvaluationID {
		t.Fatal("re-evaluation writes a new row; it must not overwrite the old verdict")
	}
	if current.Overall != "not_met" || current.SupersededAt != nil {
		t.Errorf("the current revision is the newest and is not superseded, got %+v", current)
	}

	// The quoted verdict is still readable, and reading it cannot be mistaken for
	// reading the standing one.
	status, old := c.getEvaluation(t, "/runs/"+runID+"/evaluation?revision="+first.EvaluationID)
	if status != http.StatusOK {
		t.Fatalf("the superseded revision must still be readable, got %d", status)
	}
	if old.Overall != "met" || old.SupersededAt == nil {
		t.Errorf("the old revision keeps its verdict and is stamped superseded, got %+v", old)
	}
	if old.JudgePromptVersion != "judge-run@v1" {
		t.Errorf("a verdict only means something next to the prompt it was reached under, got %q",
			old.JudgePromptVersion)
	}

	resp, err := c.Get(c.base + "/runs/" + runID + "/evaluation/revisions")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var list struct {
		Revisions []struct {
			EvaluationID       string  `json:"evaluation_id"`
			Overall            string  `json:"overall"`
			JudgePromptVersion string  `json:"judge_prompt_version"`
			SupersededAt       *string `json:"superseded_at"`
		} `json:"revisions"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if len(list.Revisions) != 2 {
		t.Fatalf("both judgements are in the history, got %d", len(list.Revisions))
	}
	if list.Revisions[0].EvaluationID != current.EvaluationID {
		t.Error("the history is newest first")
	}
	standing := 0
	for _, r := range list.Revisions {
		if r.SupersededAt == nil {
			standing++
		}
	}
	if standing != 1 {
		t.Errorf("exactly one revision is the standing one, got %d", standing)
	}
}

// --- feedback (EVAL-001 clause 4) ---------------------------------------------

func TestFeedbackIsRecordedAndCanBeChanged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-feedback")
	skillID := seedSkill(t, pool, c.workspaceID, "feedback")
	runID, _ := seedEvaluatableRun(t, pool, c.workspaceID, skillID)
	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "failed", Reason: "no"},
			{CriterionID: "c2", Result: "failed", Reason: "no"},
		},
		Overall: "not_met", Summary: "nothing was produced",
	}, "judge-run@2026-08-17")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}

	status, body := c.putJSON(t, "/runs/"+runID+"/evaluation/feedback", `{"helpful":false,"comment":"missed the point"}`)
	if status != http.StatusOK {
		t.Fatalf("PUT feedback: got %d (%s)", status, body.Error)
	}
	if body.Feedback == nil || body.Feedback.Helpful || body.Feedback.Comment != "missed the point" {
		t.Errorf("feedback was not recorded: %+v", body.Feedback)
	}

	// A user is allowed to change their mind; the second answer replaces the first
	// rather than being appended.
	_, body = c.putJSON(t, "/runs/"+runID+"/evaluation/feedback", `{"helpful":true}`)
	if body.Feedback == nil || !body.Feedback.Helpful || body.Feedback.Comment != "" {
		t.Errorf("resending replaces the previous answer: %+v", body.Feedback)
	}

	if status, _ := c.putJSON(t, "/runs/"+runID+"/evaluation/feedback", `{}`); status != http.StatusBadRequest {
		t.Errorf("`helpful` is required, got %d", status)
	}
}

func (c *client) putJSON(t *testing.T, path, body string) (int, evaluationBody) {
	t.Helper()
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		c.base+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out evaluationBody
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out
}

// --- scope and absence (iron rule 3, WS-006) ----------------------------------

func TestEvaluationsAreInvisibleAcrossWorkspacesAndAbsenceIsA404(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	owner := a.login(t, "eval-scope-owner")
	stranger := a.login(t, "eval-scope-stranger")
	skillID := seedSkill(t, pool, owner.workspaceID, "scoped")
	runID, _ := seedEvaluatableRun(t, pool, owner.workspaceID, skillID)

	// 「未評估」 is a state of its own, and a blank body is what a UI renders as a
	// pass — so an unevaluated run is 404 rather than an empty evaluation.
	for _, path := range []string{
		"/runs/" + runID + "/evaluation",
		"/runs/" + runID + "/evaluation/revisions",
	} {
		if status, _ := owner.getEvaluation(t, path); status != http.StatusNotFound {
			t.Errorf("%s on an unevaluated run: got %d, want 404", path, status)
		}
	}

	a.evaluations.Judge = judgeServer(t, llmclient.JudgeVerdict{
		CriterionResults: []llmclient.CriterionVerdict{
			{CriterionID: "c1", Result: "passed", Reason: "yes"},
			{CriterionID: "c2", Result: "passed", Reason: "yes"},
		},
		Overall: "met", Summary: "done",
	}, "judge-run@2026-08-17")
	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, owner.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}

	if status, _ := owner.getEvaluation(t, "/runs/"+runID+"/evaluation"); status != http.StatusOK {
		t.Fatalf("the owner reads their own evaluation: got %d", status)
	}
	for _, path := range []string{
		"/runs/" + runID + "/evaluation",
		"/runs/" + runID + "/evaluation/revisions",
	} {
		if status, _ := stranger.getEvaluation(t, path); status != http.StatusNotFound {
			t.Errorf("%s for a stranger: got %d, want 404 (existence is private)", path, status)
		}
	}
	if status, _ := stranger.putJSON(t, "/runs/"+runID+"/evaluation/feedback", `{"helpful":true}`); status != http.StatusNotFound {
		t.Errorf("a stranger cannot leave feedback on somebody else's verdict: got %d", status)
	}

	// Anonymous callers get 401, not data.
	anon := &http.Client{}
	resp, err := anon.Get(a.URL + "/runs/" + runID + "/evaluation") //nolint:noctx // test client
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("no session: got %d, want 401", resp.StatusCode)
	}
}

// --- a run with nothing asked of it -------------------------------------------

func TestARunWithNoAcceptanceCriteriaIsUndeterminedAndNeverReachesTheJudge(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	c := a.login(t, "eval-no-criteria")
	skillID := seedSkill(t, pool, c.workspaceID, "no-criteria")
	// seedRun's snapshot carries an empty acceptance_criteria array.
	runID := seedRun(t, pool, c.workspaceID, skillID)
	for _, status := range []string{"provisioning", "preparing", "running", "evaluating", "succeeded"} {
		if _, err := pool.Exec(context.Background(),
			`UPDATE runs SET status = $2::run_status,
			 finished_at = CASE WHEN $2 = 'succeeded' THEN now() ELSE finished_at END
			 WHERE id = $1`, mustUUID(t, runID), status); err != nil {
			t.Fatal(err)
		}
	}

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{}`)
	}))
	defer srv.Close()
	a.evaluations.Judge = &llmclient.Client{BaseURL: srv.URL}

	if err := a.evaluations.Evaluate(context.Background(),
		mustUUID(t, c.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	if called {
		t.Error("a run with no criteria has nothing for a judge to answer, and must not be paid for")
	}
	_, body := c.getEvaluation(t, "/runs/"+runID+"/evaluation")
	if body.Status != "completed" || body.Overall != "undetermined" {
		t.Errorf("nothing asked means nothing to have met, got status=%q overall=%q", body.Status, body.Overall)
	}
	if len(body.CriterionResults) != 0 {
		t.Errorf("no criteria, no criterion results, got %d", len(body.CriterionResults))
	}
	// The row completed without the judge ever being reached, and on a `completed`
	// row these columns mean "what produced this verdict". Read from the columns
	// rather than the body because the body cannot tell NULL from '': a model name
	// here would be a finished report claiming a metered call that never happened.
	var judgeModel, judgePromptVersion *string
	if err := pool.QueryRow(context.Background(),
		`SELECT judge_model, judge_prompt_version FROM evaluations WHERE run_id = $1`,
		mustUUID(t, runID)).Scan(&judgeModel, &judgePromptVersion); err != nil {
		t.Fatal(err)
	}
	if judgeModel != nil || judgePromptVersion != nil {
		t.Errorf("no judge ran, so the verdict's judge columns must be NULL, got %q / %q",
			derefTest(judgeModel), derefTest(judgePromptVersion))
	}
}

func derefTest(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// --- statics -----------------------------------------------------------------

func strPtrTest(s string) *string { return &s }

// evalPackageInterfaceCheck keeps the read handler and the worker pointed at the
// same service type; a split would let the API serve a shape the worker never
// writes.
var _ = eval.Service{}
