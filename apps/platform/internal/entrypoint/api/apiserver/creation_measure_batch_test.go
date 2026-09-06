package apiserver_test

// 02:GEN-012 / 05 R-45 — the paid measurement harness for the interactive
// creation journey against the single-shot generate path. 15 multi-turn
// sessions (5 text, 5 diagram, 5 reference), the same 15 tasks through
// versions.GenerateSkill once each. See
// docs/plans/mvp/m5/creation-measure/README.md for how to run this and what
// the owner still has to do afterward (read the drafts for "kept" — this
// harness cannot do that; "met" is filled automatically when the run stage
// below is configured, subject to a person's override in met_by_owner).
//
// Usage (spends money — one command runs everything):
//
//	CREATION_MEASURE_CORPUS    docs/plans/mvp/m5/gen-modes-batch/corpus.json
//	CREATION_MEASURE_DIAGRAMS  dir with D01.png... (pwsh .../draw.ps1 -Corpus ... -OutDir ...)
//	CREATION_MEASURE_OUT       dir for results.json and *.SKILL.md
//	SKILLHUB_E2E_LLM_URL       a running apps/llm pointed at a real gateway
//	LITELLM_API_KEY            the creation gateway key (see with-service-key.mjs)
//
// Optional run stage (fills the "met" column, 02:GEN-012): set
// SKILLHUB_E2E_SANDBOX_URL and the rest of gen009_baseline_test.go's paid-run
// env (SKILLHUB_E2E_SANDBOX_TOKEN, OBJSTORE_ENDPOINT/ACCESS_KEY/SECRET_KEY,
// SKILLHUB_E2E_PUBLIC_HOST, SKILLHUB_MODEL_GATEWAY_URL/_KEY). Unset, this
// harness behaves exactly as it did before this stage existed.

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/ArthurC02/skillhub/apps/platform/internal/creator/creation"
	identity "github.com/ArthurC02/skillhub/apps/platform/internal/creator/workspace"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/api/apiserver"
	"github.com/ArthurC02/skillhub/apps/platform/internal/entrypoint/worker"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/integration/llmclient"
	"github.com/ArthurC02/skillhub/apps/platform/internal/foundation/storage/objstore"
	"github.com/ArthurC02/skillhub/apps/platform/internal/shared/skillpkg"
	ingest "github.com/ArthurC02/skillhub/apps/platform/internal/skill/admission"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/evidence"
	"github.com/ArthurC02/skillhub/apps/platform/internal/trial/execution"
)

// creationMeasureLimits is 05 R-45's ruled values (裁定 2026-09-06).
func creationMeasureLimits() creation.Limits {
	return creation.Limits{
		MaxCostUSD: 1, MaxCallCostUSD: .1, MaxSteps: 24, MaxToolCalls: 8,
		CallTimeout: 90 * time.Second, SessionTimeout: 72 * time.Hour,
		Retention: 30 * 24 * time.Hour, MaxOutputTokens: 16000,
	}
}

// creationMeasureLoopBudget bounds one interactive session's driver loop: a
// stuck session (a model that never confirms, never drafts) must stop rather
// than spend the whole per-session ceiling in a tight loop.
const creationMeasureLoopBudget = 12

// sessionRow is one interactive session's outcome (results.json "interactive").
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
	// The optional run stage (SKILLHUB_E2E_SANDBOX_URL configured): a Run of
	// the materialized candidate against its own Test Case, the attach_run
	// observation fed back to the session, and one more step to see whether
	// the model revised its draft. Left at their zero values when the stage is
	// not configured, exactly as before this stage existed.
	RunStatus  string `json:"run_status,omitempty"`
	EvalStatus string `json:"eval_status,omitempty"`
	Overall    string `json:"overall,omitempty"`
	// Met is filled automatically from Overall once EvalStatus reaches a
	// finished status ("completed" or "failed"); left null with MetNote saying
	// why when the run stage is not configured or did not reach a verdict.
	Met     *bool  `json:"met"`
	MetNote string `json:"met_note,omitempty"`
	// RevisedAfterRun is set after the post-attach_run step: true when the
	// draft's content hash changed, false when the model kept the same draft.
	RevisedAfterRun *bool `json:"revised_after_run,omitempty"`
	// The revised draft's own trial (05 R-45, 2026-09-06 ruling: `met` counts
	// within one revision round). Filled only when the model revised, the
	// revision materialized as a new candidate and that candidate ran.
	RevisedOverall string `json:"revised_overall,omitempty"`
	RevisedMet     *bool  `json:"revised_met,omitempty"`
	RevisedNote    string `json:"revised_note,omitempty"`
	// Rounds is how many trials ran (1 = the candidate only); MetRound is the
	// first round whose trial was "met", 0 when none was. The owner's product
	// shape (2026-09-06): every round runs a trial and brings suggestions back
	// until the person accepts; `met` stands in for "acceptable" here.
	Rounds   int `json:"rounds,omitempty"`
	MetRound int `json:"met_round,omitempty"`
	// KeptByOwner is left null for the owner to fill in after a person has
	// read the SKILL.md — 05 R-45's other half this harness cannot produce on
	// its own. MetByOwner stays for a person to overrule the automatic Met.
	MetByOwner  *bool `json:"met_by_owner"`
	KeptByOwner *bool `json:"kept_by_owner"`
}

// singleShotRow is one single-shot generation's outcome ("single_shot").
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
	// The run stage's automatic tally over the text and reference sessions
	// (05 R-45, 2026-09-06 ruling): MetDenominator is how many reached a
	// verdict, MetFirstCount how many were "met" on the first trial, MetCount
	// how many were "met" on the first trial or on the revised draft's trial
	// (within one revision round). Diagram sessions are experimental and
	// tallied apart. All stay 0 when the run stage is not configured.
	MetFirstCount         int `json:"met_first_count"`
	MetCount              int `json:"met_count"`
	MetDenominator        int `json:"met_denominator"`
	DiagramMetCount       int `json:"diagram_met_count"`
	DiagramMetDenominator int `json:"diagram_met_denominator"`
}
type creationMeasureResults struct {
	Interactive []sessionRow              `json:"interactive"`
	SingleShot  []singleShotRow           `json:"single_shot"`
	Thresholds  creationMeasureThresholds `json:"thresholds"`
	Summary     creationMeasureSummary    `json:"summary"`
}

// percentile is the nearest-rank percentile over a sorted copy of xs; p in
// [0,100]. Good enough at the sizes here (15 sessions, up to a few dozen
// per-call seconds), not a statistics library.
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

// measureTask is one of the 15 tasks driven both ways.
type measureTask struct {
	ID          string
	Kind        string // "text" | "diagram" | "reference"
	Description string
	Diagram     *ingest.GenerateDiagram
	// ReferenceMD is the reference skill's SKILL.md, imported per-session so
	// each session's reference resolves against its own workspace.
	ReferenceMD string
}

// creationMessage posts a "message" command, unlike creationAct which never
// carries a message body.
func creationMessage(t *testing.T, c *client, v creation.View, message string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "message", "message": message,
	}, 200)
}

// creationAttachRun sends the "attach_run" command creationAct does not cover
// (it carries a run_id, not a content_hash).
func creationAttachRun(t *testing.T, c *client, v creation.View, runID string) creation.View {
	t.Helper()
	return creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
		"command_id": creationID(t), "expected_revision": v.Revision, "kind": "attach_run", "run_id": runID,
	}, 200)
}

// trialRun carries what the optional paid run stage needs across sessions:
// the real object store the sandbox reads packages from, and the pool for
// the one query neither the creation nor the run API exposes (a version's
// package_object_key).
type trialRun struct {
	store *objstore.Client
	pool  *pgxpool.Pool
}

// withTrialRunning gates 05 R-45's optional "met" stage on
// SKILLHUB_E2E_SANDBOX_URL, exactly as gen009_baseline_test.go gates its own
// paid run. Unset, it returns nil and a is untouched — the harness measures
// exactly as it did before this stage existed. Set, it wires a's run service
// to a real object store and sandbox provider, opens a trace listener the
// sandbox host can reach, and starts a worker running the real judge —
// reusing gen009's own helpers (objstoreBucket, startWorkerWith,
// waitForTerminalSoft, waitForEvaluation) rather than copying them.
func withTrialRunning(t *testing.T, a *api, pool *pgxpool.Pool, llmURL string, traceSigner *trace.Signer) *trialRun {
	t.Helper()
	sandboxURL := os.Getenv("SKILLHUB_E2E_SANDBOX_URL")
	if sandboxURL == "" {
		return nil
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
	a.runs.Gateway = run.GatewayFromEnv()
	if a.runs.Gateway == nil {
		t.Fatal("SKILLHUB_MODEL_GATEWAY_URL / _KEY are required alongside SKILLHUB_E2E_SANDBOX_URL")
	}
	a.runs.PollInterval = time.Second
	a.runs.MaxAttempts = 1
	a.runs.TraceSigner = traceSigner

	// The sandbox host pushes trace back, and httptest binds loopback. Same
	// second listener gen009 and the e2e test use, for the same reason.
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

	// The judge is the worker's, not the API's, same reasoning as gen009: a
	// read must never pay for a model call, so this is set on a copy.
	judging := *a.evaluations
	judging.Judge = &llmclient.Client{BaseURL: llmURL, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	startWorkerWith(t, a.runs, &judging)

	return &trialRun{store: store, pool: pool}
}

// attachTrialRun is the optional stage 05 R-45 needs for "met" (02:GEN-012):
// it puts the materialized candidate's package into the real object store,
// starts a Run against the candidate's own Test Case the way gen009 does
// (fixture.startNoFatal, waitForTerminalSoft, waitForEvaluation), folds the
// verdict into row, then feeds the run back into the session with attach_run
// and runs one more step to see whether the model revised its draft. Never
// t.Fatal's on a run/eval problem — one session's sandbox trouble must not
// cost the other rows their spend — recording the reason on row.MetNote
// instead.
// trialOutcome is one Run of a candidate against its own Test Case: the run's
// terminal status, the evaluation's status and overall, the run id, and a note
// saying why there is no verdict when there is none.
type trialOutcome struct {
	runID, runStatus, evalStatus, overall, note string
	met                                         *bool
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
	switch ev.Status {
	case "completed", "failed":
		met := ev.Overall == "met"
		out.met = &met
	default:
		out.note = "evaluation did not finish: " + ev.Status
	}
	return out
}

// attachTrialRun runs the candidate, feeds the observation back, lets the
// model revise, materializes the revision as a new candidate and runs that
// too — up to CREATION_MEASURE_ROUNDS trials (default 3), stopping at the
// first "met" (05 R-45, 2026-09-06: `met` counts within the revision rounds;
// the first trial is reported beside it).
func attachTrialRun(t *testing.T, a *api, ctx context.Context, c *client, s *creation.Service, trial *trialRun, v creation.View, row sessionRow, outDir string) (sessionRow, creation.View) {
	t.Helper()
	rounds := 3
	if n, err := strconv.Atoi(os.Getenv("CREATION_MEASURE_ROUNDS")); err == nil && n > 0 {
		rounds = n
	}
	last := trialCandidate(t, a, ctx, c, trial, v.Snapshot.Candidate)
	row.RunStatus, row.EvalStatus, row.Overall, row.Met, row.MetNote = last.runStatus, last.evalStatus, last.overall, last.met, last.note
	if last.runID == "" {
		return row, v
	}
	row.Rounds = 1
	if last.met != nil && *last.met {
		row.MetRound = 1
	}
	for round := 2; round <= rounds && row.MetRound == 0; round++ {
		beforeHash := ""
		if v.Snapshot.Draft != nil {
			beforeHash = v.Snapshot.Draft.ContentHash
		}
		v = creationAttachRun(t, c, v, last.runID)
		answered := 0
		// A nudge (unchanged draft, missing diagram node) re-queues the step,
		// a revision is followed by its validation step, and a review that
		// moves the fix into the criteria or the sample comes back as a
		// confirmation (prompt v11) which this harness grants as the person
		// would. Bounded by MaxNudges plus a few settling steps (run j left
		// every session queued at the third step).
		for i := 0; i < creation.MaxNudges+10; i++ {
			switch v.State {
			case "queued":
				v = creationStep(t, s, v)
				row.ModelCalls++
				continue
			case "waiting_confirmation":
				if v.Snapshot.PendingAction == "" {
					break
				}
				v = creationAct(t, c, v, v.Snapshot.PendingAction)
				row.AutoConfirms++
				continue
			case "waiting_input":
				// The questions after an unmet trial (05 R-47's owner note):
				// this harness answers as a person who wants the criteria met.
				if answered >= 2 {
					break
				}
				answered++
				row.Clarifications++
				v = creationMessage(t, c, v, "照沒過的條件改草稿；條件或範例輸入驗不到的，就改條件或範例輸入。")
				continue
			}
			break
		}
		afterHash := ""
		if v.Snapshot.Draft != nil {
			afterHash = v.Snapshot.Draft.ContentHash
		}
		revised := beforeHash != afterHash
		if round == 2 {
			row.RevisedAfterRun = &revised
		}
		if !revised {
			break
		}
		if v.State != "draft_ready" {
			row.RevisedNote = "revised draft did not settle: " + v.State
			break
		}
		// A new hash cleared the candidate; materialize builds a new version of
		// the same Skill (ADR-003: a revision is a new version).
		v = creationAct(t, c, v, "materialize")
		if v.Snapshot.Draft != nil {
			dumpDraftMD(t, outDir, row.ID, fmt.Sprintf("interactive-r%d", round), v.Snapshot.Draft.Skill.Name, v.Snapshot.Draft.Skill.Description, v.Snapshot.Draft.Skill.Body)
		}
		last = trialCandidate(t, a, ctx, c, trial, v.Snapshot.Candidate)
		row.RevisedOverall, row.RevisedMet, row.RevisedNote = last.overall, last.met, last.note
		if last.runID == "" {
			break
		}
		row.Rounds = round
		if last.met != nil && *last.met {
			row.MetRound = round
		}
	}
	return row, v
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

func TestCreationMeasureFifteenSessionsAgainstSingleShot(t *testing.T) {
	corpusPath := os.Getenv("CREATION_MEASURE_CORPUS")
	diagramDir := os.Getenv("CREATION_MEASURE_DIAGRAMS")
	outDir := os.Getenv("CREATION_MEASURE_OUT")
	base := os.Getenv("SKILLHUB_E2E_LLM_URL")
	gatewayKey := os.Getenv("LITELLM_API_KEY")
	if corpusPath == "" || diagramDir == "" || outDir == "" || base == "" || gatewayKey == "" {
		t.Skip("set CREATION_MEASURE_CORPUS, CREATION_MEASURE_DIAGRAMS, CREATION_MEASURE_OUT, SKILLHUB_E2E_LLM_URL and LITELLM_API_KEY; this test spends money")
	}
	raw, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var corpus modesCorpus
	if err := json.Unmarshal(raw, &corpus); err != nil {
		t.Fatal(err)
	}
	// The 15-task shape needs 5 diagrams and 10 references; a corpus with
	// neither (creation-measure/corpus-fetch.json) runs every reference entry
	// as a text task — the fetch path has no diagram or reference half.
	textOnly := len(corpus.Diagram) < 5 || len(corpus.Reference) < 10
	if textOnly && len(corpus.Reference) == 0 {
		t.Fatal("corpus has no tasks")
	}
	pool := requireDB(t)

	llm := &llmclient.Client{BaseURL: base, Token: os.Getenv("LLM_SERVICE_TOKEN")}
	limits := creationMeasureLimits()
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
	// A fixed secret, same shape as authz_integration_test.go's default
	// harness: unused unless withTrialRunning wires it onto the run service
	// below, harmless otherwise.
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
		Server: server, auth: app.Auth, app: app, packages: packages, handler: handler,
		versions: app.Versions, runs: app.RunSvc, evaluations: app.EvalSvc, traceSigner: traceSigner,
	}
	ctx := context.Background()
	trial := withTrialRunning(t, a, pool, base, traceSigner)

	// Build the 15 tasks: text = reference[0..4] (description only), diagram =
	// diagram[0..4], reference = reference[5..9] (description + its own
	// reference skill).
	var tasks []measureTask
	if textOnly {
		for _, r := range corpus.Reference {
			tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description})
		}
	}
	for i := 0; i < 5 && !textOnly; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "text", Description: r.Description})
	}
	for i := 0; i < 5 && !textOnly; i++ {
		d := corpus.Diagram[i]
		ext, mediaType := d.Media, "image/png"
		if ext == "jpg" {
			mediaType = "image/jpeg"
		}
		img, err := os.ReadFile(filepath.Join(diagramDir, d.ID+"."+ext))
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, measureTask{ID: d.ID, Kind: "diagram", Diagram: &ingest.GenerateDiagram{MediaType: mediaType, Data: img}})
	}
	for i := 5; i < 10 && !textOnly; i++ {
		r := corpus.Reference[i]
		tasks = append(tasks, measureTask{ID: r.ID, Kind: "reference", Description: r.Description, ReferenceMD: r.Reference.SkillMD})
	}

	var results creationMeasureResults
	results.Thresholds = creationMeasureThresholds{
		FormatPassMin: 14, MetMin: 6, KeptMin: 12,
		CostMedianMax: 0.5, P50SecondsMax: 60, P95SecondsMax: 90,
	}
	flush := func() {
		data, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(outDir, "results.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}

	for _, task := range tasks {
		row := runInteractiveSession(t, a, set.Creation, ctx, task, limits, outDir, trial)
		results.Interactive = append(results.Interactive, row)
		flush()
		t.Logf("interactive %s (%s): state=%s draft=%v cost=%s met=%s revised_met=%s calls=%d", task.ID, task.Kind, row.FinalState, row.Draft, costLabel(row.CostUSD), metLabel(row), revisedMetLabel(row), row.ModelCalls)
	}
	for _, task := range tasks {
		row := runSingleShot(t, a, ctx, task, outDir)
		results.SingleShot = append(results.SingleShot, row)
		flush()
		t.Logf("single-shot %s (%s): generated=%v attempts=%d cost=%s", task.ID, task.Kind, row.Generated, row.Attempts, costLabel(row.CostUSD))
	}

	for _, row := range results.Interactive {
		if row.Draft && !row.Blocked {
			results.Summary.FormatPass++
		}
		if row.Met != nil {
			within := row.MetRound > 0
			if row.Kind == "diagram" {
				results.Summary.DiagramMetDenominator++
				if within {
					results.Summary.DiagramMetCount++
				}
				continue
			}
			results.Summary.MetDenominator++
			if *row.Met {
				results.Summary.MetFirstCount++
			}
			if within {
				results.Summary.MetCount++
			}
		}
	}
	for _, row := range results.SingleShot {
		if row.Generated {
			results.Summary.FormatPass++
		}
	}
	var costs, allSeconds []float64
	for _, row := range results.Interactive {
		if row.CostUSD != nil {
			costs = append(costs, *row.CostUSD)
		}
		allSeconds = append(allSeconds, row.SecondsPerCall...)
	}
	results.Summary.CostMedian = median(costs)
	results.Summary.P50Seconds = percentile(allSeconds, 50)
	results.Summary.P95Seconds = percentile(allSeconds, 95)
	flush()

	if len(results.Interactive) != len(tasks) || len(results.SingleShot) != len(tasks) {
		t.Fatalf("expected %d+%d rows, got %d+%d", len(tasks), len(tasks), len(results.Interactive), len(results.SingleShot))
	}
}

// dumpDraftMD writes a generated skill's markdown (frontmatter + body) to
// <outDir>/<id>-<suffix>.SKILL.md, for the person who has to read 30 of these
// for "kept" (05 R-45).
func dumpDraftMD(t *testing.T, outDir, id, suffix, name, description, body string) {
	t.Helper()
	md := "---\nname: " + name + "\ndescription: " + description + "\n---\n\n" + body
	if err := os.WriteFile(filepath.Join(outDir, id+"-"+suffix+".SKILL.md"), []byte(md), 0o600); err != nil {
		t.Fatal(err)
	}
}

// runInteractiveSession drives one multi-turn session to a terminal state (or
// until the loop/message budget runs out), materializing a draft if one is
// reached, and dumps the resulting draft.
func runInteractiveSession(t *testing.T, a *api, s *creation.Service, ctx context.Context, task measureTask, limits creation.Limits, outDir string, trial *trialRun) sessionRow {
	t.Helper()
	row := sessionRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-"+strings.ToLower(task.ID))

	// A session created with a message is already queued, and Act refuses every
	// command but cancel while a step is queued (409). The diagram session starts
	// empty so the upload is its first input; the reference session runs its
	// first step before the references are selected.
	initialMessage := task.Description
	if task.Kind == "diagram" {
		initialMessage = ""
	}
	v := creationPost(t, c, "/creation-sessions", map[string]any{
		"id": creationID(t), "message": initialMessage, "budget_usd": limits.MaxCostUSD,
	}, 200)
	// The transcript is what explains a row that never reached a draft; the
	// summary line cannot. Written on every exit path of this function.
	defer func() {
		data, err := json.MarshalIndent(map[string]any{"state": v.State, "brief": v.Snapshot.Brief, "acceptance_criteria": v.Snapshot.AcceptanceCriteria, "messages": v.Snapshot.Messages}, "", "  ")
		if err == nil {
			_ = os.WriteFile(filepath.Join(outDir, task.ID+"-interactive.transcript.json"), data, 0o600)
		}
	}()

	if task.Kind == "diagram" {
		encoded := base64.StdEncoding.EncodeToString(task.Diagram.Data)
		v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "diagram",
			"diagram": map[string]string{"media_type": task.Diagram.MediaType, "data": encoded},
		}, 200)
		row.ModelCalls++ // the diagram action is one synchronous model call, outside the queued loop below
	}
	if task.Kind == "reference" {
		refID, _ := importFiles(t, a, testPool, c, map[string]string{"SKILL.md": task.ReferenceMD})
		// The first step may be a catalog search that re-queues (run j R10,
		// 2026-09-06: the flagship searched first and select_references hit
		// 409 on a queued session). Step until the session waits.
		for v.State == "queued" {
			v = creationStep(t, s, v)
			row.ModelCalls++
		}
		v = creationPost(t, c, "/creation-sessions/"+v.ID+"/actions", map[string]any{
			"command_id": creationID(t), "expected_revision": v.Revision, "kind": "select_references",
			"reference_skill_ids": []string{refID},
		}, 200)
		v = creationAct(t, c, v, "confirm_references")
	}

	clarifications := 0
	for i := 0; i < creationMeasureLoopBudget; i++ {
		switch v.State {
		case "saved", "failed", "cancelled", "needs_reupload":
			row.FinalState = v.State
			return finishSession(t, v, row, outDir)
		case "queued":
			start := time.Now()
			v = creationStep(t, s, v)
			row.SecondsPerCall = append(row.SecondsPerCall, time.Since(start).Seconds())
			row.ModelCalls++
			if v.Snapshot.SpentUSD != nil {
				row.CostUSD = v.Snapshot.SpentUSD
			}
			row.UsageUnknown = v.Snapshot.UsageUnknown
			row.ToolCalls = v.Snapshot.ToolCalls
		case "waiting_confirmation":
			kind := v.Snapshot.PendingAction
			if kind == "" {
				row.FinalState = v.State
				row.Error = "waiting_confirmation with no pending action"
				return finishSession(t, v, row, outDir)
			}
			v = creationAct(t, c, v, kind)
			row.AutoConfirms++
		case "waiting_input":
			if clarifications >= 2 {
				row.FinalState = v.State
				return finishSession(t, v, row, outDir)
			}
			clarifications++
			row.Clarifications++
			v = creationMessage(t, c, v, "請依合理假設補上缺的資訊，然後繼續。")
		case "draft_ready":
			v = creationAct(t, c, v, "materialize")
			row.FinalState = v.State
			row = finishSession(t, v, row, outDir)
			if trial != nil {
				// v is reassigned so the deferred transcript dump sees the run
				// observation and the review step (run e's transcripts stopped
				// before attach_run and could not explain revised_after_run).
				row, v = attachTrialRun(t, a, ctx, c, s, trial, v, row, outDir)
			}
			return row
		default:
			row.FinalState = v.State
			row.Error = "unexpected state: " + v.State
			return finishSession(t, v, row, outDir)
		}
		row.Turns++
	}
	row.FinalState = v.State
	row.Error = "loop budget exhausted"
	return finishSession(t, v, row, outDir)
}

// finishSession fills in the draft-derived fields from the session's final
// snapshot and dumps the draft body, if any.
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

// runSingleShot calls versions.GenerateSkill once for the same task, the
// comparison arm (02:GEN-012).
func runSingleShot(t *testing.T, a *api, ctx context.Context, task measureTask, outDir string) singleShotRow {
	t.Helper()
	row := singleShotRow{ID: task.ID, Kind: task.Kind}
	c := a.login(t, "creation-measure-single-"+strings.ToLower(task.ID))
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

// costLabel and metLabel keep the progress log readable: a nil pointer printed
// with %v is an address, and the run stage's outcome or its reason is what
// somebody watching a paid run wants to see per session.
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
