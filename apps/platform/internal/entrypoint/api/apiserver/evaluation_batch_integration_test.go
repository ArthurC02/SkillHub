package apiserver_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/persistence/pgconv"
	run "github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution/providertest"
	eval "github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

const doesNotAsk = `[{"id":"c1","text":"answers without asking","check":{"kind":"does_not_ask"}}]`

func batchBody(versionID string, trials int, testCaseIDs ...string) string {
	ids, _ := json.Marshal(testCaseIDs)
	return fmt.Sprintf(`{"skill_version_id":%q,"test_case_ids":%s,"trials":%d}`, versionID, ids, trials)
}

func (f fixture) forTestCase(testCaseID string) fixture {
	f.testCaseID = testCaseID
	return f
}

func batchTrialRuns(t *testing.T, pool *pgxpool.Pool, batchID string) map[string]string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `
		SELECT test_case_id::text || '#' || trial, COALESCE(run_id::text, '')
		FROM evaluation_batch_items WHERE batch_id = $1`, mustUUID(t, batchID))
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	runs := map[string]string{}
	for rows.Next() {
		var key, runID string
		if err := rows.Scan(&key, &runID); err != nil {
			t.Fatal(err)
		}
		runs[key] = runID
	}
	return runs
}

func started(runs map[string]string) []string {
	var keys []string
	for key, runID := range runs {
		if runID != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func TestABatchNeedsEveryTestCaseConfirmedAndStartsItsFirstTrialWhileTheRestWait(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch")
	second := seedTestCase(t, pool, f.workspaceID, f.skillID)
	f.confirmPermissions(t)
	path := "/skills/" + f.skillID + "/evaluation-batches"

	code, body := f.doJSON(t, http.MethodPost, path, batchBody(f.versionID, 2, f.testCaseID, second))
	if ids, _ := body["unconfirmed_test_case_ids"].([]any); code != http.StatusUnprocessableEntity || len(ids) != 1 || ids[0] != second {
		t.Fatalf("one test case unconfirmed: got %d %v, want 422 naming only %s", code, body, second)
	}

	f.forTestCase(second).confirmPermissions(t)
	code, body = f.doJSON(t, http.MethodPost, path, batchBody(f.versionID, 2, f.testCaseID, second))
	if code != http.StatusCreated || body["status"] != "launching" || body["trials"] != float64(2) {
		t.Fatalf("create: got %d %v, want 201 launching with 2 trials", code, body)
	}
	batchID := body["id"].(string)

	runs := batchTrialRuns(t, pool, batchID)
	if len(runs) != 4 {
		t.Fatalf("batch holds %d trials, want 2 test cases x 2 trials", len(runs))
	}
	if got := started(runs); len(got) != 1 || got[0] != f.testCaseID+"#1" {
		t.Fatalf("started trials = %v, want only the first trial of the first test case", got)
	}

	if err := a.batches.AdvanceBatches(context.Background(), mustUUID(t, f.workspaceID)); err != nil {
		t.Fatal(err)
	}
	if got := started(batchTrialRuns(t, pool, batchID)); len(got) != 1 {
		t.Errorf("advancing while the run slot is taken started %v, want still one", got)
	}

	code, list := f.doJSON(t, http.MethodGet, path, "")
	if batches, _ := list["batches"].([]any); code != http.StatusOK || len(batches) != 1 {
		t.Errorf("list: got %d %v, want the one batch", code, list)
	}
}

func TestABatchWhoseEveryTrialStartedIsLaunched(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-single")
	f.confirmPermissions(t)

	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches", batchBody(f.versionID, 1, f.testCaseID))
	if code != http.StatusCreated || body["status"] != "launched" {
		t.Fatalf("a one-trial batch: got %d %v, want 201 launched", code, body)
	}
}

func runsOfTestCase(t *testing.T, pool *pgxpool.Pool, testCaseID string) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), `
		SELECT count(*) FROM runs r JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
		WHERE s.test_case_id = $1`, mustUUID(t, testCaseID)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestATrialWhoseRunWasAlreadyStartedUnderItsKeyIsLinkedToThatRunNeverASecond(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-race")
	f.confirmPermissions(t)
	ctx := context.Background()

	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches", batchBody(f.versionID, 2, f.testCaseID))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, body)
	}
	batchID := body["id"].(string)
	first := batchTrialRuns(t, pool, batchID)[f.testCaseID+"#1"]
	mustExec(t, pool, `UPDATE runs SET status = 'cancelled', finished_at = now() WHERE id = $1`, mustUUID(t, first))
	before := runsOfTestCase(t, pool, f.testCaseID)

	ports := a.batches.Ports
	start := ports.StartRun
	var earlier pgtype.UUID
	ports.StartRun = func(ctx context.Context, b eval.BatchRun) (pgtype.UUID, error) {
		if !earlier.Valid {
			var err error
			if earlier, err = start(ctx, b); err != nil {
				t.Fatalf("the earlier advance could not start the trial: %v", err)
			}
		}
		return start(ctx, b)
	}
	replaying := &eval.BatchService{Pool: pool, Ports: ports}
	if err := replaying.AdvanceBatches(ctx, mustUUID(t, f.workspaceID)); err != nil {
		t.Fatalf("starting a trial a second time is not an error: %v", err)
	}

	if after := runsOfTestCase(t, pool, f.testCaseID); after != before+1 {
		t.Errorf("runs for the test case went from %d to %d, want exactly one new run", before, after)
	}
	if got := batchTrialRuns(t, pool, batchID)[f.testCaseID+"#2"]; got != pgconv.UUIDString(earlier) {
		t.Errorf("trial 2 is linked to %q, want the run the earlier advance started, %s", got, pgconv.UUIDString(earlier))
	}
}

func TestAnOriginKeyThatStartedOneSkillVersionCannotStartAnother(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-origin-reuse")
	hash := f.confirmPermissions(t)
	ctx := context.Background()
	params := run.CreateParams{
		WorkspaceID: mustUUID(t, f.workspaceID), Actor: mustUUID(t, f.userID), SkillID: mustUUID(t, f.skillID),
		VersionID: mustUUID(t, f.versionID), TestCaseID: mustUUID(t, f.testCaseID),
		ConfirmedSummaryHash: hash, OriginKey: "caller/one",
	}
	created, err := a.runs.Create(ctx, params)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}
	again, err := a.runs.Create(ctx, params)
	if err != nil || again.ID != created.ID {
		t.Fatalf("same key again: got %v, %v; want the run %v back", again.ID, err, created.ID)
	}

	params.VersionID = mustUUID(t, seedSkillVersion(t, pool, f.workspaceID, f.skillID))
	if _, err := a.runs.Create(ctx, params); !errors.Is(err, run.ErrOriginKeyReused) {
		t.Fatalf("same key, another version: %v, want ErrOriginKeyReused", err)
	}
}

func dispatchedRequest(t *testing.T, pool *pgxpool.Pool, fake *providertest.Fake, runID string) run.RunRequest {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if req, ok := fake.Request(runID, 1); ok {
			return req
		}
		time.Sleep(50 * time.Millisecond)
	}
	var status, reason string
	_ = pool.QueryRow(context.Background(), `SELECT status::text, COALESCE(status_reason, '') FROM runs WHERE id = $1`,
		mustUUID(t, runID)).Scan(&status, &reason)
	t.Fatalf("run %s was never dispatched; it is %s (%s)", runID, status, reason)
	return run.RunRequest{}
}

func grantsSkillPackage(req run.RunRequest) bool {
	for _, g := range req.ObjectGrants {
		if g.Purpose == "skill_package" {
			return true
		}
	}
	return false
}

func bothArmsStarted(t *testing.T, a *api, pool *pgxpool.Pool, workspaceID, batchID string) (withSkill, withheld string) {
	t.Helper()
	ctx := context.Background()
	for deadline := time.Now().Add(30 * time.Second); withheld == "" && time.Now().Before(deadline); {
		if err := a.batches.AdvanceBatches(ctx, mustUUID(t, workspaceID)); err != nil {
			t.Fatal(err)
		}
		_ = pool.QueryRow(ctx, `
			SELECT COALESCE(max(run_id::text) FILTER (WHERE NOT skill_withheld), ''),
			       COALESCE(max(run_id::text) FILTER (WHERE skill_withheld), '')
			FROM evaluation_batch_items WHERE batch_id = $1`, mustUUID(t, batchID)).Scan(&withSkill, &withheld)
		time.Sleep(50 * time.Millisecond)
	}
	return withSkill, withheld
}

func TestABaselineBatchRunsEveryTrialAgainWithTheSkillWithheld(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	fake, _ := withProvider(t, a, pool, providertest.Plan{CreatingPolls: 1, RunningPolls: 1})
	f := newFixture(t, a, pool, "alice-batch-baseline")
	f.confirmPermissions(t)
	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches",
		`{"skill_version_id":"`+f.versionID+`","test_case_ids":["`+f.testCaseID+`"],"trials":1,"with_baseline":true}`)
	if code != http.StatusCreated || body["with_baseline"] != true {
		t.Fatalf("create: got %d %v, want 201 with a baseline", code, body)
	}
	batchID := body["id"].(string)

	withSkill, withheld := bothArmsStarted(t, a, pool, f.workspaceID, batchID)
	if withheld == "" || withSkill == "" || withheld == withSkill {
		t.Fatalf("trial runs: with %q, withheld %q; want two distinct runs", withSkill, withheld)
	}

	if req := dispatchedRequest(t, pool, fake, withSkill); !grantsSkillPackage(req) || req.SkillVersion.ObjectKey == "" {
		t.Errorf("the trial with the skill was dispatched without its package: %+v", req.SkillVersion)
	}
	if req := dispatchedRequest(t, pool, fake, withheld); grantsSkillPackage(req) || req.SkillVersion.ObjectKey != "" {
		t.Errorf("the baseline trial was handed the skill: %+v", req.SkillVersion)
	}
	if _, view := f.doJSON(t, http.MethodGet, "/runs/"+withheld, ""); view["skill_withheld"] != true {
		t.Errorf("the baseline run reads %v, want it marked as run without the skill", view["skill_withheld"])
	}
	if _, view := f.doJSON(t, http.MethodGet, "/runs/"+withSkill, ""); view["skill_withheld"] != nil {
		t.Errorf("the ordinary run reads skill_withheld=%v, want it absent", view["skill_withheld"])
	}

	_, report := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/evaluation-batches/"+batchID, "")
	if without, _ := report["without_skill"].(map[string]any); without == nil || len(without["test_cases"].([]any)) != 1 {
		t.Errorf("report without_skill = %v, want the withheld arm with its one test case", report["without_skill"])
	}
	if report["skill_effect"] == nil {
		t.Error("report has no skill_effect, want the paired difference")
	}
}

func TestARefusalThatWaitingCannotFixStopsTheBatch(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-stop")
	f.confirmPermissions(t)
	ctx := context.Background()

	code, body := f.doJSON(t, http.MethodPost, "/skills/"+f.skillID+"/evaluation-batches", batchBody(f.versionID, 2, f.testCaseID))
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, body)
	}
	batchID := body["id"].(string)
	first := batchTrialRuns(t, pool, batchID)[f.testCaseID+"#1"]
	mustExec(t, pool, `UPDATE runs SET status = 'cancelled', finished_at = now() WHERE id = $1`, mustUUID(t, first))
	mustExec(t, pool, `DELETE FROM run_permission_confirmations WHERE workspace_id = $1`, mustUUID(t, f.workspaceID))

	if err := a.batches.AdvanceBatches(ctx, mustUUID(t, f.workspaceID)); err != nil {
		t.Fatal(err)
	}
	code, report := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/evaluation-batches/"+batchID, "")
	batch, _ := report["batch"].(map[string]any)
	if code != http.StatusOK || batch["status"] != "stopped" || batch["stop_reason"] != "permissions_unconfirmed" {
		t.Fatalf("after the confirmation vanished: got %d %v, want stopped for permissions_unconfirmed", code, batch)
	}
	if got := started(batchTrialRuns(t, pool, batchID)); len(got) != 1 {
		t.Errorf("a stopped batch started %v, want only its first trial", got)
	}
	trial := report["test_cases"].([]any)[0].(map[string]any)["trials"].([]any)[0].(map[string]any)
	if trial["outcome"] != "not_evaluated" {
		t.Errorf("the cancelled first trial = %v, want not_evaluated", trial)
	}
}

func TestTheBatchReportCountsEachConversationAtItsEnd(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-report")
	ctx := context.Background()
	ws := mustUUID(t, f.workspaceID)
	answered, unasked := f.testCaseID, seedTestCase(t, pool, f.workspaceID, f.skillID)
	untouched := seedTestCase(t, pool, f.workspaceID, f.skillID)

	metRun := seedFinishedRun(t, pool, f, finishedRun{testCaseID: unasked, criteria: doesNotAsk, contentHash: "sha256:first"})
	notMetRun := seedFinishedRun(t, pool, f, finishedRun{testCaseID: unasked, criteria: asksFirst, contentHash: "sha256:edited"})
	asking := seedAskingRun(t, pool, f, "小孩算不算人數？")
	continued := seedAnsweredRound(t, pool, f, asking)
	waiting := seedAskingRun(t, pool, f, "幾點出發？")
	for _, runID := range []string{metRun, notMetRun, continued} {
		seedFinalOutput(t, pool, f.workspaceID, runID, "共四位。")
	}
	for _, runID := range []string{metRun, notMetRun, asking, continued, waiting} {
		if err := a.evaluations.Evaluate(ctx, ws, mustUUID(t, runID)); err != nil {
			t.Fatalf("evaluate %s: %v", runID, err)
		}
	}

	var batchID string
	if err := pool.QueryRow(ctx, `
		WITH b AS (
			INSERT INTO evaluation_batches (workspace_id, skill_id, skill_version_id, trials, status)
			VALUES ($1, $2, $3, 2, 'launched') RETURNING id),
		i AS (
			INSERT INTO evaluation_batch_items (batch_id, workspace_id, test_case_id, position, trial, summary_hash, run_id)
			SELECT b.id, $1, t.test_case_id, t.position, t.trial, 'sha256:summary', t.run_id
			FROM b, (VALUES ($4::uuid, 0, 1, $5::uuid), ($4, 0, 2, $6), ($7, 1, 1, $8), ($7, 1, 2, $9),
			                ($10, 2, 1, NULL), ($10, 2, 2, NULL)) AS t (test_case_id, position, trial, run_id))
		SELECT id::text FROM b`,
		ws, mustUUID(t, f.skillID), mustUUID(t, f.versionID),
		mustUUID(t, unasked), mustUUID(t, metRun), mustUUID(t, notMetRun),
		mustUUID(t, answered), mustUUID(t, asking), mustUUID(t, waiting),
		mustUUID(t, untouched),
	).Scan(&batchID); err != nil {
		t.Fatal(err)
	}

	code, report := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/evaluation-batches/"+batchID, "")
	if code != http.StatusOK {
		t.Fatalf("report: %d %v", code, report)
	}
	cases := report["test_cases"].([]any)
	want := []batchTestCaseWant{
		{unasked, []string{"met", "not_met"}, 1, 2, true, false, true},
		{answered, []string{"met", "awaiting_answer"}, 1, 1, true, nil, false},
		{untouched, []string{"pending", "pending"}, 0, 0, nil, nil, false},
	}
	if len(cases) != len(want) {
		t.Fatalf("report has %d test cases, want %d", len(cases), len(want))
	}
	for i, w := range want {
		w.check(t, cases[i].(map[string]any))
	}
	if end := cases[1].(map[string]any)["trials"].([]any)[0].(map[string]any)["end_run_id"]; end != continued {
		t.Errorf("the answered trial ends at %v, want the continuing run %s", end, continued)
	}

	checkTwoOfThreePassed(t, report)
}

func checkTwoOfThreePassed(t *testing.T, report map[string]any) {
	t.Helper()
	interval, _ := report["pass_rate_95"].(map[string]any)
	if report["passed"] != float64(2) || report["judged"] != float64(3) ||
		report["test_cases_decided"] != float64(1) || report["test_cases_all_passed"] != float64(0) ||
		!near(report["pass_rate"], 2.0/3) || !near(interval["low"], 0.2077) || !near(interval["high"], 0.9385) {
		t.Errorf("summary = passed %v judged %v decided %v all %v rate %v interval %v, "+
			"want 2 of 3 with the Wilson interval [0.2077, 0.9385]",
			report["passed"], report["judged"], report["test_cases_decided"], report["test_cases_all_passed"],
			report["pass_rate"], interval)
	}
}

type seededTrial struct {
	testCaseID string
	runID      string
}

func seedBatchOfOneTrial(t *testing.T, pool *pgxpool.Pool, f fixture, trials ...seededTrial) string {
	t.Helper()
	ctx := context.Background()
	var batchID string
	if err := pool.QueryRow(ctx, `
		INSERT INTO evaluation_batches (workspace_id, skill_id, skill_version_id, trials, status)
		VALUES ($1, $2, $3, 1, 'launched') RETURNING id::text`,
		mustUUID(t, f.workspaceID), mustUUID(t, f.skillID), mustUUID(t, f.versionID)).Scan(&batchID); err != nil {
		t.Fatal(err)
	}
	for position, trial := range trials {
		mustExec(t, pool, `
			INSERT INTO evaluation_batch_items (batch_id, workspace_id, test_case_id, position, trial, summary_hash, run_id)
			VALUES ($1, $2, $3, $4, 1, 'sha256:summary', $5)`,
			mustUUID(t, batchID), mustUUID(t, f.workspaceID), mustUUID(t, trial.testCaseID), position, mustUUID(t, trial.runID))
	}
	return batchID
}

func (f fixture) evaluatedRun(t *testing.T, a *api, pool *pgxpool.Pool, testCaseID string, met bool) string {
	t.Helper()
	criteria := asksFirst
	if met {
		criteria = doesNotAsk
	}
	runID := seedFinishedRun(t, pool, f, finishedRun{testCaseID: testCaseID, criteria: criteria, contentHash: "sha256:" + testCaseID})
	seedFinalOutput(t, pool, f.workspaceID, runID, "共四位。")
	if err := a.evaluations.Evaluate(context.Background(), mustUUID(t, f.workspaceID), mustUUID(t, runID)); err != nil {
		t.Fatal(err)
	}
	return runID
}

func TestTwoBatchesAreComparedOnlyOnTheTestCasesBothJudged(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-batch-compare")
	improved, steady := f.testCaseID, seedTestCase(t, pool, f.workspaceID, f.skillID)
	added := seedTestCase(t, pool, f.workspaceID, f.skillID)
	waiting := seedTestCase(t, pool, f.workspaceID, f.skillID)
	asking := seedFinishedRun(t, pool, f, finishedRun{testCaseID: waiting, criteria: asksFirst, contentHash: "sha256:waiting"})
	seedQuestions(t, pool, f.workspaceID, asking, "幾點出發？")

	baseline := seedBatchOfOneTrial(t, pool, f,
		seededTrial{improved, f.evaluatedRun(t, a, pool, improved, false)},
		seededTrial{steady, f.evaluatedRun(t, a, pool, steady, true)},
		seededTrial{waiting, f.evaluatedRun(t, a, pool, waiting, true)})
	candidate := seedBatchOfOneTrial(t, pool, f,
		seededTrial{improved, f.evaluatedRun(t, a, pool, improved, true)},
		seededTrial{steady, f.evaluatedRun(t, a, pool, steady, true)},
		seededTrial{added, f.evaluatedRun(t, a, pool, added, true)},
		seededTrial{waiting, asking})

	path := "/skills/" + f.skillID + "/evaluation-batches/" + candidate + "/comparison?against=" + baseline
	code, body := f.doJSON(t, http.MethodGet, path, "")
	interval, _ := body["difference_95"].(map[string]any)
	if code != http.StatusOK || body["paired"] != float64(2) || body["unpaired"] != float64(2) ||
		body["candidate_better"] != float64(1) || body["tied"] != float64(1) || body["baseline_better"] != float64(0) ||
		!near(body["difference"], 0.5) || !near(interval["low"], -0.48) || !near(interval["high"], 1.48) {
		t.Fatalf("comparison: got %d %v, want 2 pairs (an awaiting trial leaves its test case unpaired), one better and one tied, 0.5 within [-0.48, 1.48]", code, body)
	}

	if code, _ := f.doJSON(t, http.MethodGet, "/skills/"+f.skillID+"/evaluation-batches/"+candidate+"/comparison", ""); code != http.StatusNotFound {
		t.Errorf("no baseline named: got %d, want 404", code)
	}
}

type batchTestCaseWant struct {
	testCaseID     string
	outcomes       []string
	passed, judged float64
	passAtK        any
	passAllK       any
	varied         bool
}

func (w batchTestCaseWant) check(t *testing.T, got map[string]any) {
	t.Helper()
	var outcomes []string
	for _, trial := range got["trials"].([]any) {
		outcomes = append(outcomes, trial.(map[string]any)["outcome"].(string))
	}
	if got["test_case_id"] != w.testCaseID || fmt.Sprint(outcomes) != fmt.Sprint(w.outcomes) ||
		got["passed"] != w.passed || got["judged"] != w.judged || got["pass_at_k"] != w.passAtK ||
		got["pass_all_k"] != w.passAllK || got["prompt_varied_across"] != w.varied {
		t.Errorf("test case = %v, want %+v", got, w)
	}
}

func near(got any, want float64) bool {
	value, ok := got.(float64)
	return ok && math.Abs(value-want) < 0.0005
}

func TestTheRunListShowsEachRunsCurrentVerdictAndSaysNotEvaluatedOtherwise(t *testing.T) {
	pool := requireDB(t)
	a := newAPI(t, pool)
	f := newFixture(t, a, pool, "alice-run-list-verdicts")
	judged := seedFinishedRun(t, pool, f, finishedRun{testCaseID: f.testCaseID, criteria: doesNotAsk, contentHash: "sha256:judged"})
	unjudged := seedFinishedRun(t, pool, f, finishedRun{testCaseID: f.testCaseID, criteria: doesNotAsk, contentHash: "sha256:unjudged"})
	mustExec(t, pool, `
		INSERT INTO evaluations (workspace_id, run_id, status, overall, evidence_complete)
		VALUES ($1, $2, 'completed', 'met', true)`, mustUUID(t, f.workspaceID), mustUUID(t, judged))

	code, body := f.doJSON(t, http.MethodGet, "/runs?test_case_id="+f.testCaseID, "")
	if code != http.StatusOK {
		t.Fatalf("list: got %d %v", code, body)
	}
	verdicts := map[string]any{}
	for _, item := range body["runs"].([]any) {
		row := item.(map[string]any)
		verdicts[row["run_id"].(string)] = row["evaluation"].(map[string]any)["value"]
	}
	if verdicts[judged] != "met" || verdicts[unjudged] != "not_evaluated" {
		t.Errorf("verdicts = %v, want %s met and %s not_evaluated", verdicts, judged, unjudged)
	}
}
