package apiserver_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/wiring"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/improvement"
)

func creationMeasureLimits() creation.Limits {
	return creation.Limits{
		MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 24, MaxToolCalls: 8,
		CallTimeout: 90 * time.Second, SessionTimeout: 72 * time.Hour,
		Retention: 30 * 24 * time.Hour, MaxOutputTokens: 16000,
	}
}

const creationMeasureLoopBudget = 12

const creationMeasureRevisionReply = "照沒過的條件改草稿。範例輸入驗不到的條件，就補範例輸入讓它驗得到；條件本身不要改寫或刪掉。"

type sessionRow struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind"`
	Turns          int       `json:"turns"`
	ModelCalls     int       `json:"model_calls"`
	ToolCalls      int       `json:"tool_calls"`
	AutoConfirms   int       `json:"auto_confirms"`
	Clarifications int       `json:"clarifications"`
	CostUSD        *float64  `json:"cost_usd,omitempty"`
	UsageUnknown   bool      `json:"usage_unknown"`
	SecondsPerCall []float64 `json:"seconds_per_call"`
	P50Seconds     float64   `json:"p50_seconds"`
	P95Seconds     float64   `json:"p95_seconds"`
	FinalState     string    `json:"final_state"`
	Draft          bool      `json:"draft"`
	Blocked        bool      `json:"blocked"`
	CriteriaCount  int       `json:"criteria_count"`
	TestCaseID     bool      `json:"test_case_id"`
	Error          string    `json:"error,omitempty"`

	RunStatus  string `json:"run_status,omitempty"`
	EvalStatus string `json:"eval_status,omitempty"`
	Overall    string `json:"overall,omitempty"`

	Met     *bool  `json:"met"`
	MetNote string `json:"met_note,omitempty"`

	RevisedAfterRun *bool `json:"revised_after_run,omitempty"`

	RevisedOverall string `json:"revised_overall,omitempty"`
	RevisedMet     *bool  `json:"revised_met,omitempty"`
	RevisedNote    string `json:"revised_note,omitempty"`

	SearchHit *bool `json:"search_hit,omitempty"`

	CatalogOffers int `json:"catalog_offers,omitempty"`

	DuplicateOffers int    `json:"duplicate_offers,omitempty"`
	SearchNote      string `json:"search_note,omitempty"`

	Rounds   int `json:"rounds,omitempty"`
	MetRound int `json:"met_round,omitempty"`

	ChallengeCases  int `json:"challenge_cases,omitempty"`
	ChallengePassed int `json:"challenge_passed,omitempty"`

	CriteriaChangedBeforeMet *bool `json:"criteria_changed_before_met,omitempty"`

	HoldoutMet    *bool    `json:"holdout_met,omitempty"`
	HoldoutPassed int      `json:"holdout_passed,omitempty"`
	HoldoutCases  int      `json:"holdout_cases,omitempty"`
	HoldoutNotes  []string `json:"holdout_notes,omitempty"`

	MetByOwner  *bool `json:"met_by_owner"`
	KeptByOwner *bool `json:"kept_by_owner"`
}

type singleShotRow struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Generated bool     `json:"generated"`
	Blocked   bool     `json:"blocked"`
	Attempts  int      `json:"attempts"`
	CostUSD   *float64 `json:"cost_usd,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type creationMeasureThresholds struct {
	FormatPassMin int     `json:"format_pass_min"`
	MetMin        int     `json:"met_min"`
	KeptMin       int     `json:"kept_min"`
	CostMedianMax float64 `json:"cost_median_max"`
	P50SecondsMax float64 `json:"p50_seconds_max"`
	P95SecondsMax float64 `json:"p95_seconds_max"`
}
type creationMeasureSummary struct {
	FormatPass int     `json:"format_pass"`
	CostMedian float64 `json:"cost_median"`
	P50Seconds float64 `json:"p50_seconds"`
	P95Seconds float64 `json:"p95_seconds"`

	MetFirstCount         int `json:"met_first_count"`
	MetCount              int `json:"met_count"`
	MetOnChangedCriteria  int `json:"met_on_changed_criteria"`
	HoldoutMetCount       int `json:"holdout_met_count"`
	HoldoutDenominator    int `json:"holdout_denominator"`
	MetDenominator        int `json:"met_denominator"`
	DiagramMetCount       int `json:"diagram_met_count"`
	DiagramMetDenominator int `json:"diagram_met_denominator"`
}

func (s *creationMeasureSummary) countJudged(row sessionRow) {
	within := row.MetRound > 0
	if row.Kind == "diagram" {
		s.DiagramMetDenominator++
		if within {
			s.DiagramMetCount++
		}
		return
	}
	s.MetDenominator++
	if *row.Met {
		s.MetFirstCount++
	}
	if within {
		s.MetCount++
	}
	if row.CriteriaChangedBeforeMet != nil && *row.CriteriaChangedBeforeMet {
		s.MetOnChangedCriteria++
	}
	if row.HoldoutMet == nil {
		return
	}
	s.HoldoutDenominator++
	if *row.HoldoutMet {
		s.HoldoutMetCount++
	}
}

type creationMeasureResults struct {
	Interactive []sessionRow              `json:"interactive"`
	SingleShot  []singleShotRow           `json:"single_shot"`
	Thresholds  creationMeasureThresholds `json:"thresholds"`
	Summary     creationMeasureSummary    `json:"summary"`
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]float64(nil), xs...)
	sort.Float64s(sorted)
	idx := int(p/100*float64(len(sorted)-1) + 0.5)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}
func median(xs []float64) float64 { return percentile(xs, 50) }

type measureTask struct {
	ID          string
	Kind        string
	Description string
	Diagram     *ingest.GenerateDiagram

	DiagramNodes []string
	ReferenceMD  string
	Holdout      []holdoutCase
}

type holdoutCase struct {
	Name     string   `json:"name"`
	Prompt   string   `json:"prompt"`
	Criteria []string `json:"criteria"`
}

func allHoldoutsMet(results []*bool) bool {
	if len(results) == 0 {
		return false
	}
	for _, met := range results {
		if met == nil || !*met {
			return false
		}
	}
	return true
}

func TestAHoldoutCountsAsMetOnlyWhenEveryCaseRanAndMet(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		name    string
		results []*bool
		want    bool
	}{
		{"no case ran", nil, false},
		{"every case met", []*bool{&yes, &yes}, true},
		{"one case failed", []*bool{&yes, &no}, false},
		{"one case never ran", []*bool{&yes, nil}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := allHoldoutsMet(tc.results); got != tc.want {
				t.Fatalf("allHoldoutsMet = %v, want %v", got, tc.want)
			}
		})
	}
}

type cannedJudge struct {
	judgement *eval.Judgement
	err       error
}

func (c cannedJudge) JudgeRun(context.Context, eval.JudgeRequest) (*eval.Judgement, error) {
	return c.judgement, c.err
}

func TestTheMeasureHarnessKeepsEveryJudgeRequestItSends(t *testing.T) {
	cases := []struct {
		name      string
		judge     cannedJudge
		wantError string
	}{
		{"the judge answers", cannedJudge{judgement: &eval.Judgement{Overall: "met"}}, ""},
		{"the judge fails", cannedJudge{err: fmt.Errorf("judge unavailable")}, "judge unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			req := eval.JudgeRequest{RunID: "run-1", UserPrompt: "summarise this", FinalOutput: "a summary"}
			got, err := recordingJudge{next: tc.judge, outDir: dir}.JudgeRun(context.Background(), req)
			if got != tc.judge.judgement || !errors.Is(err, tc.judge.err) {
				t.Fatalf("the recorder changed what the judge returned: %v, %v", got, err)
			}
			raw, rerr := os.ReadFile(filepath.Join(dir, "judge-run-1.json"))
			if rerr != nil {
				t.Fatal(rerr)
			}
			var record judgeRecord
			if err := json.Unmarshal(raw, &record); err != nil {
				t.Fatal(err)
			}
			if record.Request.UserPrompt != "summarise this" || record.Request.FinalOutput != "a summary" || record.Error != tc.wantError {
				t.Fatalf("record = %+v", record)
			}
			if (record.Judgement == nil) != (tc.judge.judgement == nil) {
				t.Fatalf("judgement recorded = %v, want %v", record.Judgement, tc.judge.judgement)
			}
		})
	}
}

func createHoldoutCase(t *testing.T, c *client, skillID string, hc holdoutCase) (string, error) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"skill_id": skillID, "name": hc.Name, "user_prompt": hc.Prompt})
	code, created := postJSON(t, c, "/test-cases", string(body))
	id, _ := created["test_case_id"].(string)
	if code != http.StatusCreated || id == "" {
		return "", fmt.Errorf("POST /test-cases: %d %v", code, created)
	}
	for _, criterion := range hc.Criteria {
		text, _ := json.Marshal(map[string]string{"text": criterion})
		if code, out := postJSON(t, c, "/test-cases/"+id+"/criteria", string(text)); code != http.StatusCreated {
			return "", fmt.Errorf("POST criterion: %d %v", code, out)
		}
	}
	return id, nil
}

func runHoldout(t *testing.T, a *api, ctx context.Context, c *client, trial *trialRun, v creation.View, task measureTask, row sessionRow, outDir string) sessionRow {
	t.Helper()
	if len(task.Holdout) == 0 || v.Snapshot.Candidate == nil {
		return row
	}
	var results []*bool
	for i, hc := range task.Holdout {
		id, err := createHoldoutCase(t, c, v.Snapshot.Candidate.SkillID, hc)
		if err != nil {
			row.HoldoutNotes = append(row.HoldoutNotes, hc.Name+": "+err.Error())
			results = append(results, nil)
			continue
		}
		candidate := *v.Snapshot.Candidate
		candidate.TestCaseID = id
		out := trialCandidate(t, a, ctx, c, trial, &candidate)
		writeTrialRecord(t, outDir, fmt.Sprintf("%s-holdout-%d", row.ID, i+1), 0, out.record)
		if out.note != "" {
			row.HoldoutNotes = append(row.HoldoutNotes, hc.Name+": "+out.note)
		}
		results = append(results, out.met)
		if out.met != nil && *out.met {
			row.HoldoutPassed++
		}
	}
	row.HoldoutCases = len(results)
	met := allHoldoutsMet(results)
	row.HoldoutMet = &met
	return row
}

func creationMessage(t *testing.T, c *client, v creation.View, message string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": message,
	}, 200)
}

func creationAttachRun(t *testing.T, c *client, v creation.View, runID string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "attach_run", "run_id": runID,
	}, 200)
}

type trialRun struct {
	store *objstore.Client
	pool  *pgxpool.Pool
}

type recordingJudge struct {
	next   eval.Judge
	outDir string
}

type judgeRecord struct {
	Request   eval.JudgeRequest `json:"request"`
	Judgement *eval.Judgement   `json:"judgement"`
	Error     string            `json:"error,omitempty"`
}

func (j recordingJudge) JudgeRun(ctx context.Context, req eval.JudgeRequest) (*eval.Judgement, error) {
	got, err := j.next.JudgeRun(ctx, req)
	record := judgeRecord{Request: req, Judgement: got}
	if err != nil {
		record.Error = err.Error()
	}
	raw, _ := json.MarshalIndent(record, "", "  ")
	if werr := os.WriteFile(filepath.Join(j.outDir, "judge-"+req.RunID+".json"), raw, 0o644); werr != nil {
		fmt.Fprintf(os.Stderr, "judge record for run %s not written: %v\n", req.RunID, werr)
	}
	return got, err
}

func withTrialRunning(t *testing.T, a *api, pool *pgxpool.Pool, llmURL string, traceSigner *trace.Signer, outDir string) *trialRun {
	t.Helper()
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if sandboxURL == "" {
		return nil
	}
	gatewayURL := os.Getenv("SKILLHUB_E2E_GATEWAY_URL")
	if gatewayURL == "" {
		t.Fatal("SKILLHUB_E2E_GATEWAY_URL is required so trial runs use the sandbox-routable gateway")
	}
	ctx := context.Background()
	store, err := objstore.New(
		os.Getenv("OBJSTORE_ENDPOINT"), os.Getenv("OBJSTORE_ACCESS_KEY"),
		os.Getenv("OBJSTORE_SECRET_KEY"), objstoreBucket(), false,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureBucket(ctx); err != nil {
		t.Fatal(err)
	}
	a.runs.Store = store
	a.runs.Providers = run.NewRegistry(run.NewProvider(
		"self_hosted", sandboxURL, os.Getenv("SKILLHUB_E2E_SANDBOX_TOKEN")))
	a.runs.Gateway = wiring.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required alongside SKILLHUB_E2E_SANDBOX_URL")
	}
	a.runs.Deployment.GatewayURL = gatewayURL
	a.runs.Deployment.Model = os.Getenv("SKILLHUB_RUN_MODEL")
	a.runs.Deployment.MinimumIsolation = run.WeakIsolation
	a.runs.PollInterval = time.Second
	a.runs.MaxAttempts = 1
	a.runs.TraceSigner = traceSigner

	public := httptest.NewUnstartedServer(a.handler)
	listener, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatal(err)
	}
	public.Listener = listener
	public.Start()
	t.Cleanup(public.Close)
	a.runs.TraceIngestBaseURL = fmt.Sprintf("http://%s:%d",
		os.Getenv("SKILLHUB_E2E_PUBLIC_HOST"), listener.Addr().(*net.TCPAddr).Port)

	judging := *a.evaluations
	judging.Judge = recordingJudge{
		next:   eval.JudgeOrNone(&llmclient.Client{BaseURL: llmURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}),
		outDir: outDir,
	}
	startWorkerWith(t, a.runs, &judging)

	return &trialRun{store: store, pool: pool}
}

type trialOutcome struct {
	runID, runStatus, evalStatus, overall, note string
	met                                         *bool
	record                                      trialRecord
}

type trialCriterion struct {
	Text string `json:"text"`
}

type trialRecord struct {
	Round       int              `json:"round"`
	SampleInput string           `json:"sample_input"`
	Criteria    []trialCriterion `json:"acceptance_criteria"`
	FinalOutput string           `json:"final_output"`
	Artifacts   []string         `json:"artifacts"`
	Evaluation  evaluationBody   `json:"evaluation"`
	ReadErrors  []string         `json:"read_errors,omitempty"`
}

func (r trialRecord) criteriaTexts() []string {
	texts := make([]string, 0, len(r.Criteria))
	for _, c := range r.Criteria {
		texts = append(texts, c.Text)
	}
	return texts
}

func readTrialRecord(ctx context.Context, pool *pgxpool.Pool, runID string, ev evaluationBody) trialRecord {
	record := trialRecord{Evaluation: ev}
	failed := func(what string, err error) {
		record.ReadErrors = append(record.ReadErrors, what+": "+err.Error())
	}
	var criteria []byte
	if err := pool.QueryRow(ctx, `
		SELECT s.user_prompt, s.acceptance_criteria
		FROM runs r JOIN test_case_snapshots s ON s.id = r.test_case_snapshot_id
		WHERE r.id = $1`, runID).Scan(&record.SampleInput, &criteria); err != nil {
		failed("test case snapshot", err)
	} else if err := json.Unmarshal(criteria, &record.Criteria); err != nil {
		failed("acceptance criteria", err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT payload->>'text' FROM trace_events
		WHERE run_id = $1 AND event_type = 'agent_output' AND payload->>'kind' = 'final'
		ORDER BY seq DESC LIMIT 1`, runID).Scan(&record.FinalOutput); err != nil {
		failed("final agent output", err)
	}
	rows, err := pool.Query(ctx, `SELECT file_name FROM artifacts WHERE run_id = $1 ORDER BY file_name`, runID)
	if err != nil {
		failed("artifacts", err)
		return record
	}
	record.Artifacts, err = pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		failed("artifacts", err)
	}
	return record
}

func writeTrialRecord(t *testing.T, outDir, id string, round int, record trialRecord) {
	t.Helper()
	record.Round = round
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, fmt.Sprintf("%s-trial-r%d.json", id, round)), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func trialCandidate(t *testing.T, a *api, ctx context.Context, c *client, trial *trialRun, candidate *creation.Candidate) trialOutcome {
	t.Helper()
	if candidate == nil {
		return trialOutcome{note: "no candidate to run"}
	}
	if candidate.TestCaseID == "" {
		return trialOutcome{note: "candidate has no test_case_id"}
	}
	var key string
	if err := trial.pool.QueryRow(ctx, "SELECT package_object_key FROM skill_versions WHERE id = $1",
		mustUUID(t, candidate.VersionID)).Scan(&key); err != nil {
		return trialOutcome{note: "package_object_key lookup: " + err.Error()}
	}
	pkg, ok := a.packages[key]
	if !ok {
		return trialOutcome{note: "the candidate package is not in the API's store under " + key}
	}
	if err := trial.store.Put(ctx, key, pkg); err != nil {
		return trialOutcome{note: "put package: " + err.Error()}
	}
	f := fixture{client: c, skillID: candidate.SkillID, versionID: candidate.VersionID, testCaseID: candidate.TestCaseID}
	code, rv := f.startNoFatal(t)
	if code != http.StatusCreated && code != http.StatusOK {
		return trialOutcome{note: fmt.Sprintf("POST run: %d %s", code, rv.Error)}
	}
	out := trialOutcome{runID: rv.RunID}
	final := waitForTerminalSoft(t, c, rv.RunID, 8*time.Minute)
	out.runStatus = final.Status
	ev := waitForEvaluation(t, c, rv.RunID, 4*time.Minute)
	out.evalStatus = ev.Status
	out.overall = ev.Overall
	out.record = readTrialRecord(ctx, trial.pool, rv.RunID, ev)
	switch ev.Status {
	case "completed", "failed":
		met := ev.Overall == "met"
		out.met = &met
	default:
		out.note = "evaluation did not finish: " + ev.Status
	}
	return out
}

type trialInputs struct {
	draftHash   string
	sampleInput string
	criteria    []string
}

func trialInputsOf(s creation.Snapshot) trialInputs {
	in := trialInputs{sampleInput: s.SampleInput, criteria: s.AcceptanceCriteria}
	if s.Draft != nil {
		in.draftHash = s.Draft.ContentHash
	}
	return in
}

func (a trialInputs) equal(b trialInputs) bool {
	return a.draftHash == b.draftHash && a.sampleInput == b.sampleInput && slices.Equal(a.criteria, b.criteria)
}

func TestTheMeasureHarnessCountsAnyTrialInputChangeAsARevision(t *testing.T) {
	draft := &creation.Draft{ContentHash: "h1"}
	base := creation.Snapshot{Draft: draft, SampleInput: "499 元", AcceptanceCriteria: []string{"a", "b"}}
	cases := []struct {
		name    string
		mutate  func(s *creation.Snapshot)
		revised bool
	}{
		{"nothing changed", func(*creation.Snapshot) {}, false},
		{"only the draft changed", func(s *creation.Snapshot) { s.Draft = &creation.Draft{ContentHash: "h2"} }, true},
		{"only the sample changed", func(s *creation.Snapshot) { s.SampleInput = "499、500、999、1000 元" }, true},
		{"only a criterion changed", func(s *creation.Snapshot) { s.AcceptanceCriteria = []string{"a", "b2"} }, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			after := base
			after.AcceptanceCriteria = slices.Clone(base.AcceptanceCriteria)
			tc.mutate(&after)
			if got := !trialInputsOf(base).equal(trialInputsOf(after)); got != tc.revised {
				t.Fatalf("revised = %v, want %v", got, tc.revised)
			}
		})
	}
}

func TestTheMeasureHarnessAttachesTheMainRunUntilItIsMetThenAnUnmetChallenge(t *testing.T) {
	cases := []struct {
		name       string
		mainMet    *bool
		challenges challengeRound
		attach     string
		met        bool
	}{
		{"main unmet, challenges unmet", boolp(false), challengeRound{cases: 2, unmetRunID: "challenge"}, "main", false},
		{"main unjudged, no challenges", nil, challengeRound{}, "main", false},
		{"main met, one challenge unmet", boolp(true), challengeRound{cases: 2, passed: 1, unmetRunID: "challenge"}, "challenge", false},
		{"main met, every challenge met", boolp(true), challengeRound{cases: 2, passed: 2}, "main", true},
		{"main met, no challenges", boolp(true), challengeRound{}, "main", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := trialRevision{last: trialOutcome{runID: "main", met: tc.mainMet}, challenges: tc.challenges}
			if got := r.runToAttach(); got != tc.attach {
				t.Errorf("attached %q, want %q", got, tc.attach)
			}
			if got := r.roundMet(); got != tc.met {
				t.Errorf("round met = %v, want %v", got, tc.met)
			}
		})
	}
}

func attachTrialRun(t *testing.T, a *api, ctx context.Context, c *client, s *creation.Service, trial *trialRun, v creation.View, row sessionRow, outDir string) (sessionRow, creation.View) {
	t.Helper()
	rounds := measureRounds(os.Getenv("CREATION_MEASURE_ROUNDS"))
	last := trialCandidate(t, a, ctx, c, trial, v.Snapshot.Candidate)
	row.RunStatus, row.EvalStatus, row.Overall, row.Met, row.MetNote = last.runStatus, last.evalStatus, last.overall, last.met, last.note
	if last.runID == "" {
		return row, v
	}
	row.Rounds = 1
	writeTrialRecord(t, outDir, row.ID, 1, last.record)
	r := &trialRevision{
		session: measureSession{t: t, a: a, ctx: ctx, c: c, s: s, trial: trial, outDir: outDir},
		row:     row, v: v, last: last, firstCriteria: last.record.criteriaTexts(),
	}
	r.challenges = r.runChallenges(1)
	if r.roundMet() {
		r.row.MetRound = 1
	}
	for round := 2; round <= rounds && r.row.MetRound == 0; round++ {
		if !r.reviseAndRun(round) {
			break
		}
	}
	return r.row, r.v
}

func measureRounds(raw string) int {
	rounds := 3
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		rounds = n
	}
	return rounds
}

type measureSession struct {
	t      *testing.T
	a      *api
	ctx    context.Context
	c      *client
	s      *creation.Service
	trial  *trialRun
	outDir string
}

type trialRevision struct {
	session       measureSession
	row           sessionRow
	v             creation.View
	last          trialOutcome
	challenges    challengeRound
	firstCriteria []string
}

type challengeRound struct {
	cases, passed int
	unmetRunID    string
}

func (r *trialRevision) runChallenges(round int) challengeRound {
	m := r.session
	var out challengeRound
	candidate := r.v.Snapshot.Candidate
	if candidate == nil {
		return out
	}
	for i, id := range candidate.ChallengeTestCaseIDs {
		challenge := *candidate
		challenge.TestCaseID = id
		trialled := trialCandidate(m.t, m.a, m.ctx, m.c, m.trial, &challenge)
		writeTrialRecord(m.t, m.outDir, fmt.Sprintf("%s-challenge-%d", r.row.ID, i+1), round, trialled.record)
		out.cases++
		switch {
		case trialled.met != nil && *trialled.met:
			out.passed++
		case out.unmetRunID == "":
			out.unmetRunID = trialled.runID
		}
	}
	r.row.ChallengeCases, r.row.ChallengePassed = out.cases, out.passed
	return out
}

func (r *trialRevision) roundMet() bool {
	return r.last.met != nil && *r.last.met && r.challenges.passed == r.challenges.cases
}

func (r *trialRevision) runToAttach() string {
	if r.last.met != nil && *r.last.met && r.challenges.unmetRunID != "" {
		return r.challenges.unmetRunID
	}
	return r.last.runID
}

func (r *trialRevision) reviseAndRun(round int) bool {
	m := r.session
	t := m.t
	before := trialInputsOf(r.v.Snapshot)
	r.v = creationAttachRun(t, m.c, r.v, r.runToAttach())
	r.v = settleRevision(m, r.v, &r.row)
	revised := !before.equal(trialInputsOf(r.v.Snapshot))
	if round == 2 {
		r.row.RevisedAfterRun = &revised
	}
	if !revised {
		return false
	}
	if r.v.State != "draft_ready" {
		r.row.RevisedNote = "revised draft did not settle: " + r.v.State
		return false
	}

	r.v = materializeThrough(t, m.c, r.v, &r.row)
	if r.v.Snapshot.Draft != nil {
		dumpDraftMD(t, m.outDir, r.row.ID, fmt.Sprintf("interactive-r%d", round), r.v.Snapshot.Draft.Skill.Name, r.v.Snapshot.Draft.Skill.Description, r.v.Snapshot.Draft.Skill.Body)
	}
	r.last = trialCandidate(t, m.a, m.ctx, m.c, m.trial, r.v.Snapshot.Candidate)
	r.row.RevisedOverall, r.row.RevisedMet, r.row.RevisedNote = r.last.overall, r.last.met, r.last.note
	if r.last.runID == "" {
		return false
	}
	r.row.Rounds = round
	writeTrialRecord(t, m.outDir, r.row.ID, round, r.last.record)
	r.challenges = r.runChallenges(round)
	if r.roundMet() {
		r.row.MetRound = round
		changed := !slices.Equal(r.firstCriteria, r.last.record.criteriaTexts())
		r.row.CriteriaChangedBeforeMet = &changed
	}
	return true
}

func settleRevision(m measureSession, v creation.View, row *sessionRow) creation.View {
	answered := 0
	for i := 0; i < creation.MaxNudges+10; i++ {
		switch {
		case v.State == "queued":
			v = creationStep(m.t, m.s, v)
			row.ModelCalls++
		case v.State == "waiting_confirmation" && v.Snapshot.PendingAction != "":
			v = creationAct(m.t, m.c, v, string(v.Snapshot.PendingAction))
			row.AutoConfirms++
		case v.State == "waiting_input" && answered < 2:
			answered++
			row.Clarifications++
			v = creationMessage(m.t, m.c, v, creationMeasureRevisionReply)
		default:
			return v
		}
	}
	return v
}

func revisedMetLabel(row sessionRow) string {
	if row.RevisedMet == nil {
		if row.RevisedNote != "" {
			return "n/a (" + row.RevisedNote + ")"
		}
		return "n/a"
	}
	return fmt.Sprintf("%v (%s, rounds=%d, met_round=%d)", *row.RevisedMet, row.RevisedOverall, row.Rounds, row.MetRound)
}

type creationMeasureEnvironment struct {
	corpusPath, diagramDir, outDir, base, gatewayKey string
}

func creationMeasureEnv(t *testing.T) creationMeasureEnvironment {
	t.Helper()
	corpusPath := os.Getenv("CREATION_MEASURE_CORPUS")
	diagramDir := os.Getenv("CREATION_MEASURE_DIAGRAMS")
	outDir := os.Getenv("CREATION_MEASURE_OUT")
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	gatewayKey := os.Getenv("LITELLM_API_KEY")
	if corpusPath == "" || diagramDir == "" || outDir == "" || base == "" || gatewayKey == "" {
		t.Skip("set CREATION_MEASURE_CORPUS, CREATION_MEASURE_DIAGRAMS, CREATION_MEASURE_OUT, SKILLHUB_E2E_LLM_URL and LITELLM_API_KEY; this test spends money")
	}
	return creationMeasureEnvironment{corpusPath: corpusPath, diagramDir: diagramDir, outDir: outDir, base: base, gatewayKey: gatewayKey}
}

func TestCreationMeasureFifteenSessionsAgainstSingleShot(t *testing.T) {
	env := creationMeasureEnv(t)
	outDir := env.outDir
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	corpus := readModesCorpus(t, env.corpusPath)

	textOnly := len(corpus.Diagram) < 5 || len(corpus.Reference) < 10
	if textOnly && len(corpus.Reference) == 0 {
		t.Fatal("corpus has no tasks")
	}
	pool := requireDB(t)

	llm := &llmclient.Client{BaseURL: env.base, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	limits := creationMeasureLimits()
	a, creator := newCreationMeasureAPI(t, pool, llm, limits, env.gatewayKey)
	ctx := context.Background()
	trial := withTrialRunning(t, a, pool, env.base, a.traceSigner, outDir)

	var tasks []measureTask
	if textOnly {
		tasks = textMeasureTasks(corpus.Reference)
	} else {
		tasks = mixedMeasureTasks(t, corpus, env.diagramDir)
	}
	tasks = onlyMeasureTasks(tasks, os.Getenv("CREATION_MEASURE_ONLY"))

	var results creationMeasureResults
	results.Thresholds = creationMeasureThresholds{
		FormatPassMin: 14, MetMin: 9, KeptMin: 12,
		CostMedianMax: 0.5, P50SecondsMax: 60, P95SecondsMax: 90,
	}

	for _, task := range tasks {
		row := runInteractiveSession(t, a, creator, ctx, task, limits, outDir, trial, llm)
		results.Interactive = append(results.Interactive, row)
		writeCreationMeasureResults(t, outDir, results)
		t.Logf("interactive %s (%s): state=%s draft=%v cost=%s met=%s revised_met=%s search_hit=%s calls=%d", task.ID, task.Kind, row.FinalState, row.Draft, costLabel(row.CostUSD), metLabel(row), revisedMetLabel(row), searchLabel(row), row.ModelCalls)
	}
	for _, task := range tasks {
		row := runSingleShot(t, a, ctx, task, outDir)
		results.SingleShot = append(results.SingleShot, row)
		writeCreationMeasureResults(t, outDir, results)
		t.Logf("single-shot %s (%s): generated=%v attempts=%d cost=%s", task.ID, task.Kind, row.Generated, row.Attempts, costLabel(row.CostUSD))
	}

	results.Summary = summarizeCreationMeasure(results.Interactive, results.SingleShot)
	writeCreationMeasureResults(t, outDir, results)

	if len(results.Interactive) != len(tasks) || len(results.SingleShot) != len(tasks) {
		t.Fatalf("expected %d+%d rows, got %d+%d", len(tasks), len(tasks), len(results.Interactive), len(results.SingleShot))
	}
}

func writeCreationMeasureResults(t *testing.T, outDir string, results creationMeasureResults) {
	t.Helper()
	data, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "results.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func summarizeCreationMeasure(interactive []sessionRow, singleShot []singleShotRow) creationMeasureSummary {
	var summary creationMeasureSummary
	for _, row := range interactive {
		if row.Draft && !row.Blocked {
			summary.FormatPass++
		}
		if row.Met != nil {
			summary.countJudged(row)
		}
	}
	for _, row := range singleShot {
		if row.Generated {
			summary.FormatPass++
		}
	}
	var costs, allSeconds []float64
	for _, row := range interactive {
		if row.CostUSD != nil {
			costs = append(costs, *row.CostUSD)
		}
		allSeconds = append(allSeconds, row.SecondsPerCall...)
	}
	summary.CostMedian = median(costs)
	summary.P50Seconds = percentile(allSeconds, 50)
	summary.P95Seconds = percentile(allSeconds, 95)
	return summary
}

func textMeasureTasks(references []modesReference) []measureTask {
	var tasks []measureTask
	for _, r := range references {
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description, Holdout: r.Holdout})
	}
	return tasks
}

func referenceMeasureTasks(references []modesReference) []measureTask {
	var tasks []measureTask
	for _, r := range references {
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "reference", Description: r.Description, ReferenceMD: r.Reference.SkillMD, Holdout: r.Holdout})
	}
	return tasks
}

func onlyMeasureTasks(tasks []measureTask, only string) []measureTask {
	if only == "" {
		return tasks
	}
	return slices.DeleteFunc(tasks, func(task measureTask) bool { return task.Kind != only })
}

func mixedMeasureTasks(t *testing.T, corpus modesCorpus, diagramDir string) []measureTask {
	t.Helper()
	tasks := textMeasureTasks(corpus.Reference[:5])
	for i := 0; i < 5; i++ {
		tasks = append(tasks, diagramMeasureTask(t, corpus.Diagram[i], diagramDir))
	}
	return append(tasks, referenceMeasureTasks(corpus.Reference[5:10])...)
}

func diagramMeasureTask(t *testing.T, d modesDiagram, diagramDir string) measureTask {
	t.Helper()
	ext, mediaType := d.Media, diagramMediaType(d.Media)
	img, err := os.ReadFile(filepath.Join(diagramDir, d.ID+"."+ext))
	if err != nil {
		t.Fatal(err)
	}
	var nodes []string
	for _, n := range d.Nodes {
		nodes = append(nodes, n.Label)
	}
	return measureTask{ID: d.ID, Kind: "diagram", Diagram: &ingest.GenerateDiagram{MediaType: mediaType, Data: img}, DiagramNodes: nodes}
}

func newCreationMeasureAPI(t *testing.T, pool *pgxpool.Pool, llm *llmclient.Client, limits creation.Limits, gatewayKey string) (*api, *creation.Service) {
	t.Helper()
	set, err := worker.BuildWorkers(pool, worker.Deps{CreationLimits: limits, LLM: llm})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.IssueKey = func(context.Context, string, string, float64, time.Duration) (string, error) {
		return gatewayKey, nil
	}
	set.Creation.RevokeKey = func(context.Context, string) error { return nil }
	transient := httptest.NewServer(set.Creation.TransientHandler("creation-measure"))
	t.Cleanup(transient.Close)
	packages := packageStore{}

	traceSigner := &trace.Signer{Secret: []byte("creation-measure-trace-secret")}
	app, err := apiserver.NewApp(apiserver.Config{
		Pool: pool, Store: packages, LLM: llm, OAuth: &identity.GitHubOAuth{}, DevLogin: true,
		GenerateExposed: true, CreationExposed: true, CreationLimits: limits,
		CreationTransient: creation.TransientClient(transient.URL, "creation-measure", 95*time.Second),
		TraceSigner:       traceSigner,
	})
	if err != nil {
		t.Fatal(err)
	}
	set.Creation.ResolveReference = app.CreationSvc.ResolveReference
	handler := app.Handler()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	a := &api{
		creditPool: pool, startingCredits: betaGrantCredits,
		Server: server, auth: app.Auth, app: app, packages: packages, handler: handler,
		versions: app.Versions, runs: app.RunSvc, evaluations: app.EvalSvc, traceSigner: traceSigner,
	}
	return a, set.Creation
}

func dumpDraftMD(t *testing.T, outDir, id, suffix, name, description, body string) {
	t.Helper()
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(outDir, id+"-"+suffix+".SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
}

func recordCatalogSearch(row *sessionRow, v creation.View, refID string) {
	offered := v.State == "waiting_confirmation" && v.Snapshot.PendingAction == "confirm_references"
	searched := v.Snapshot.CatalogChecked || offered
	for _, m := range v.Snapshot.Messages {
		if m.Role == "tool" && strings.Contains(m.Content, "目錄") {
			searched = true
		}
	}
	if !searched {
		row.SearchNote = "model did not search"
		return
	}
	hit := false
	if offered {
		for _, ref := range v.Snapshot.References {
			if ref.SkillID == refID {
				hit = true
			}
		}
	}
	row.SearchHit = &hit
}

var measureRunNonce = time.Now().UTC().Format("0102-150405")

func runInteractiveSession(t *testing.T, a *api, s *creation.Service, ctx context.Context, task measureTask, limits creation.Limits, outDir string, trial *trialRun, llm *llmclient.Client) sessionRow {
	t.Helper()
	m := &interactiveSession{
		measureSession: measureSession{t: t, a: a, ctx: ctx, s: s, trial: trial, outDir: outDir},
		task:           task,
		row:            sessionRow{ID: task.ID, Kind: task.Kind},
	}

	m.c = a.login(t, "creation-measure-"+measureRunNonce+"-"+strings.ToLower(task.ID))

	initialMessage := initialCreationMessage(task)

	refID := m.importReference(llm)
	m.v = creationPost(t, m.c, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": initialMessage, "budget_credits": int64(limits.MaxCostUSD * 1300),
	}, 200)

	defer m.writeTranscript()

	m.sendMaterials(refID)
	return m.drive()
}

type interactiveSession struct {
	measureSession
	task           measureTask
	row            sessionRow
	v              creation.View
	clarifications int
}

type sessionTurn int

const (
	turnCounted sessionTurn = iota
	turnUncounted
	turnFinished
)

func initialCreationMessage(task measureTask) string {
	if task.Kind == "diagram" {
		return ""
	}
	return task.Description
}

func draftHashOf(v creation.View) string {
	if v.Snapshot.Draft == nil {
		return ""
	}
	return v.Snapshot.Draft.ContentHash
}

func (m *interactiveSession) importReference(llm *llmclient.Client) string {
	var refID string
	if m.task.Kind == "reference" {

		refID, _ = importFilesEnriched(m.t, m.a, testPool, m.c, map[string]string{"SKILL.md": m.task.ReferenceMD}, llm)
		markCatalog(m.t, testPool, m.c.workspaceID)
	}
	return refID
}

func (m *interactiveSession) writeTranscript() {
	v := m.v
	data, err := json.MarshalIndent(map[string]any{"state": v.State, "brief": v.Snapshot.Brief, "acceptance_criteria": v.Snapshot.AcceptanceCriteria, "messages": v.Snapshot.Messages}, "", "  ")
	if err == nil {
		_ = os.WriteFile(filepath.Join(m.outDir, m.task.ID+"-interactive.transcript.json"), data, 0o600)
	}
}

func (m *interactiveSession) sendMaterials(refID string) {
	t, c := m.t, m.c
	if m.task.Kind == "diagram" {
		encoded := base64.StdEncoding.EncodeToString(m.task.Diagram.Data)
		m.v = creationPost(t, c, "/creation-sessions/"+m.v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": m.v.Revision, "kind": "diagram",
			"diagram": map[string]string{"media_type": m.task.Diagram.MediaType, "data": encoded},
		}, 200)
		m.row.ModelCalls++
	}
	if m.task.Kind == "reference" {

		for m.v.State == "queued" {
			m.v = creationStep(t, m.s, m.v)
			m.row.ModelCalls++
		}
		if os.Getenv("CREATION_MEASURE_SEARCH") == "1" {
			recordCatalogSearch(&m.row, m.v, refID)
		} else {
			m.v = creationPost(t, c, "/creation-sessions/"+m.v.ID+"/actions", map[string]any{
				"command_id": creationID(t), "expected_revision": m.v.Revision, "kind": "select_references",
				"reference_skill_ids": []string{refID},
			}, 200)
			m.v = creationAct(t, c, m.v, "confirm_references")
		}
	}
}

func (m *interactiveSession) drive() sessionRow {
	for i := 0; i < creationMeasureLoopBudget; i++ {
		switch m.turn() {
		case turnFinished:
			return m.row
		case turnUncounted:
			continue
		}
		m.row.Turns++
	}
	m.row.FinalState = m.v.State
	m.row.Error = "loop budget exhausted"
	return finishSession(m.t, m.v, m.row, m.outDir)
}

func (m *interactiveSession) turn() sessionTurn {
	switch m.v.State {
	case "saved", "failed", "cancelled", "needs_reupload":
		return m.finish("")
	case "queued":
		m.step()
		return turnCounted
	case "waiting_confirmation":
		return m.confirmPending()
	case "waiting_input":
		return m.clarify()
	case "draft_ready":
		return m.materializeAndTrial()
	default:
		return m.finish("unexpected state: " + m.v.State)
	}
}

func (m *interactiveSession) finish(problem string) sessionTurn {
	m.row.FinalState = m.v.State
	if problem != "" {
		m.row.Error = problem
	}
	m.row = finishSession(m.t, m.v, m.row, m.outDir)
	return turnFinished
}

func (m *interactiveSession) step() {
	start := time.Now()
	m.v = creationStep(m.t, m.s, m.v)
	m.row.SecondsPerCall = append(m.row.SecondsPerCall, time.Since(start).Seconds())
	m.row.ModelCalls++
	if m.v.Snapshot.SpentUSD != nil {
		m.row.CostUSD = m.v.Snapshot.SpentUSD
	}
	m.row.UsageUnknown = m.v.Snapshot.UsageUnknown
	m.row.ToolCalls = m.v.Snapshot.ToolCalls
}

func (m *interactiveSession) confirmPending() sessionTurn {
	t := m.t
	kind := m.v.Snapshot.PendingAction
	if kind == "" {
		return m.finish("waiting_confirmation with no pending action")
	}
	if kind == "confirm_references" && m.task.Kind != "reference" {

		m.row.CatalogOffers = len(m.v.Snapshot.References)
		kind = "decline_references"
	}
	if kind == creation.PendingDiagramAnswers {
		var refused string
		m.v, refused = answerDiagramUncertainties(t, m.c, m.v, m.task.DiagramNodes, &m.row)
		if refused != "" {
			return m.finish(refused)
		}
		return turnUncounted
	}

	code, body := creationPostStatus(t, m.c, "/creation-sessions/"+m.v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": m.v.Revision, "kind": kind, "content_hash": draftHashOf(m.v)})
	if code != 200 {
		return m.finish(fmt.Sprintf("%s refused: %d %s", kind, code, body))
	}
	if err := json.Unmarshal([]byte(body), &m.v); err != nil {
		t.Fatal(err)
	}
	m.row.AutoConfirms++
	return turnCounted
}

func (m *interactiveSession) clarify() sessionTurn {
	if m.clarifications >= 2 {
		return m.finish("")
	}
	m.clarifications++
	m.row.Clarifications++
	m.v = creationMessage(m.t, m.c, m.v, "請依合理假設補上缺的資訊，然後繼續。")
	return turnCounted
}

func (m *interactiveSession) materializeAndTrial() sessionTurn {
	m.v = materializeThrough(m.t, m.c, m.v, &m.row)
	m.row.FinalState = m.v.State
	m.row = finishSession(m.t, m.v, m.row, m.outDir)
	if m.trial != nil {

		m.row, m.v = attachTrialRun(m.t, m.a, m.ctx, m.c, m.s, m.trial, m.v, m.row, m.outDir)
		m.row = runHoldout(m.t, m.a, m.ctx, m.c, m.trial, m.v, m.task, m.row, m.outDir)
	}
	return turnFinished
}

func diagramAnswer(nodes []string) string {
	return "圖上的步驟依序是：" + strings.Join(nodes, " → ") + "。照圖上畫的走，圖上沒畫到的不要加。"
}

func answerDiagramUncertainties(t *testing.T, c *client, v creation.View, nodes []string, row *sessionRow) (creation.View, string) {
	t.Helper()
	if v.Snapshot.DiagramInterpretation == nil {
		return v, "answer_diagram_uncertainties pending without an interpretation"
	}
	for _, u := range v.Snapshot.DiagramInterpretation.Uncertainties {
		if u.Answer != "" {
			continue
		}
		code, body := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "answer_diagram_uncertainty",
			"diagram_uncertainty_id": u.ID, "diagram_answer": diagramAnswer(nodes),
		})
		if code != 200 {
			return v, fmt.Sprintf("answer_diagram_uncertainty refused: %d %s", code, body)
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatal(err)
		}
		row.Clarifications++
	}
	return v, ""
}

func materializeThrough(t *testing.T, c *client, v creation.View, row *sessionRow) creation.View {
	t.Helper()

	act := func(kind string) bool {
		code, body := creationPostStatus(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{"command_id": creationID(t), "expected_revision": v.Revision, "kind": kind, "content_hash": v.Snapshot.Draft.ContentHash})
		if code != 200 {
			row.Error = fmt.Sprintf("%s refused: %d %s", kind, code, body)
			return false
		}
		if err := json.Unmarshal([]byte(body), &v); err != nil {
			t.Fatal(err)
		}
		return true
	}
	if !act("materialize") {
		return v
	}
	if v.State == "waiting_confirmation" && v.Snapshot.PendingAction == "confirm_duplicate" {
		row.DuplicateOffers += len(v.Snapshot.Duplicates)
		act("confirm_duplicate")
	}
	return v
}

func finishSession(t *testing.T, v creation.View, row sessionRow, outDir string) sessionRow {
	t.Helper()
	row.CriteriaCount = len(v.Snapshot.AcceptanceCriteria)
	if v.Snapshot.Draft != nil {
		row.Draft = true
		row.Blocked = v.Snapshot.Draft.Blocked
		dumpDraftMD(t, outDir, row.ID, "interactive", v.Snapshot.Draft.Skill.Name, v.Snapshot.Draft.Skill.Description, v.Snapshot.Draft.Skill.Body)
	}
	if v.Snapshot.Candidate != nil && v.Snapshot.Candidate.TestCaseID != "" {
		row.TestCaseID = true
	}
	return row
}

func runSingleShot(t *testing.T, a *api, ctx context.Context, task measureTask, outDir string) singleShotRow {
	t.Helper()
	row := singleShotRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-single-"+measureRunNonce+"-"+strings.ToLower(task.ID))
	ws := workspaceOf(t, testPool, c)

	in := ingest.GenerateInput{TaskDescription: task.Description}
	switch task.Kind {
	case "diagram":
		in.Diagram = task.Diagram
	case "reference":
		refID, _ := importFiles(t, a, testPool, c, map[string]string{"SKILL.md": task.ReferenceMD})
		in.ReferenceSkillIDs = []pgtype.UUID{mustUUID(t, refID)}
	}
	res, err := a.versions.GenerateSkill(ctx, ws, in)
	if err != nil {
		row.Error = err.Error()
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	row.Attempts, row.CostUSD = res.Attempts, res.CostUSD
	if res.Report.Blocked {
		row.Blocked = true
		return row
	}
	row.Generated = true
	data, err := a.packages.Get(ctx, res.Version.PackageObjectKey)
	if err != nil {
		t.Logf("single-shot %s: stored package unreadable: %v", task.ID, err)
		return row
	}
	fsys, err := skillpkg.PackageFS(data)
	if err != nil {
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	md, err := fs.ReadFile(fsys, "SKILL.md")
	if err != nil {
		t.Logf("single-shot %s: %v", task.ID, err)
		return row
	}
	if err := os.WriteFile(filepath.Join(outDir, task.ID+"-single.SKILL.md"), md, 0o600); err != nil {
		t.Fatal(err)
	}
	return row
}

func costLabel(c *float64) string {
	if c == nil {
		return "unknown"
	}
	return fmt.Sprintf("$%.4f", *c)
}

func metLabel(row sessionRow) string {
	if row.Met != nil {
		return fmt.Sprintf("%v (%s)", *row.Met, row.Overall)
	}
	if row.MetNote != "" {
		return "null: " + row.MetNote
	}
	return "null"
}

func boolLabel(b *bool) string {
	if b == nil {
		return "n/a"
	}
	return fmt.Sprintf("%v", *b)
}

func searchLabel(row sessionRow) string {
	if row.SearchNote != "" {
		return "n/a (" + row.SearchNote + ")"
	}
	return boolLabel(row.SearchHit)
}
